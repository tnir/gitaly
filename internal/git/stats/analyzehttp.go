package stats

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlab.com/gitlab-org/gitaly/internal/git/pktline"
)

type Clone struct {
	URL         string
	Interactive bool
	Record      func(string, float64)

	wants []string
	get
	post
}

type get struct {
	start          time.Time
	responseHeader time.Duration
	httpStatus     int
	firstGitPacket time.Duration
	responseBody   time.Duration
	payloadSize    int64
	packets        int
	refs           int
}

// Perform does a Git HTTP clone, discarding cloned data to /dev/null.
func (st *Clone) Perform(ctx context.Context) error {

	if err := st.doGet(ctx); err != nil {
		return ctxErr(ctx, err)
	}
	if err := st.doPost(ctx); err != nil {
		return ctxErr(ctx, err)
	}

	return nil
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

/*
	"get_response_header_seconds":           "Time to Git HTTP GET response header",
	"get_http_status":                       "Git HTTP GET status code",
	"get_first_git_packet_seconds":          "Time to first Git packet in HTTP response",
	"get_response_seconds":                  "Time to complete Git HTTP GET roundtrip",
	"get_git_packets":                       "Number of Git packets in HTTP GET response",
	"get_git_packet_payload_bytes":          "Number of Git payload bytes in HTTP GET response",
	"get_advertised_refs":                   "Number of refs advertised by Git HTTP server",
	"get_wanted_refs":                       "Number of refs selected for Git HTTP clone",
	"post_response_header_seconds":          "Time to Git HTTP POST response header",
	"post_http_status":                      "Git HTTP POST status code",
	"post_nak_seconds":                      "Time to NAK Git packet in HTTP POST response",
	"post_response_seconds":                 "Time to complete Git HTTP POST roundtrip",
	"post_largest_git_packet_payload_bytes": "Largest Git packet payload in POST response",
*/

/*
	"post_first_git_packet_seconds": "Time to first Git packet in HTTP POST response",
	"post_git_packets":              "Number of Git packets in HTTP POST response",
	"post_git_payload_bytes":        "Git packet payload bytes in HTTP POST response",
*/

func (st *Clone) doGet(ctx context.Context) error {
	req, err := http.NewRequest("GET", st.URL+"/info/refs?service=git-upload-pack", nil)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	for k, v := range map[string]string{
		"User-Agent":      "gitaly-debug",
		"Accept":          "*/*",
		"Accept-Encoding": "deflate, gzip",
		"Pragma":          "no-cache",
	} {
		req.Header.Set(k, v)
	}

	st.get.start = time.Now()
	st.printInteractive("---")
	st.printInteractive("--- GET %v", req.URL)
	st.printInteractive("---")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	st.get.responseHeader = time.Since(st.get.start)
	st.get.httpStatus = resp.StatusCode
	st.printInteractive("response header: %v", resp.Header)

	body := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		body, err = gzip.NewReader(body)
		if err != nil {
			return err
		}
	}

	// Expected response:
	// - "# service=git-upload-pack\n"
	// - FLUSH
	// - "<OID> <ref> <capabilities>\n"
	// - "<OID> <ref>\n"
	// - ...
	// - FLUSH
	//
	seenFlush := false
	scanner := pktline.NewScanner(body)
	for ; scanner.Scan(); st.get.packets++ {
		if seenFlush {
			return errors.New("received packet after flush")
		}

		data := string(pktline.Data(scanner.Bytes()))
		st.get.payloadSize += int64(len(data))
		switch st.get.packets {
		case 0:
			st.get.firstGitPacket = time.Since(st.get.start)

			if data != "# service=git-upload-pack\n" {
				return fmt.Errorf("unexpected header %q", data)
			}
		case 1:
			if !pktline.IsFlush(scanner.Bytes()) {
				return errors.New("missing flush after service announcement")
			}
		default:
			if st.get.packets == 2 && !strings.Contains(data, " side-band-64k") {
				return fmt.Errorf("missing side-band-64k capability in %q", data)
			}

			if pktline.IsFlush(scanner.Bytes()) {
				seenFlush = true
				continue
			}

			split := strings.SplitN(data, " ", 2)
			if len(split) != 2 {
				continue
			}
			st.get.refs++

			if strings.HasPrefix(split[1], "refs/heads/") || strings.HasPrefix(split[1], "refs/tags/") {
				st.wants = append(st.wants, split[0])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !seenFlush {
		return errors.New("missing flush in response")
	}

	st.get.responseBody = time.Since(st.get.start)

	return nil
}

type post struct {
	start              time.Time
	responseHeader     time.Duration
	httpStatus         int
	nak                time.Duration
	multiband          map[string]*bandInfo
	responseBody       time.Duration
	status             int
	packets            int
	largestPayloadSize int
}

type bandInfo struct {
	firstPacket time.Duration
	size        int64
	packets     int
}

const (
	bandMin = 1
	bandMax = 3
)

func (st *Clone) doPost(ctx context.Context) error {
	st.multiband = make(map[string]*bandInfo)
	for i := byte(bandMin); i < bandMax; i++ {
		band, err := bandToHuman(i)
		if err != nil {
			return err
		}
		st.multiband[band] = &bandInfo{}
	}

	reqBodyRaw := &bytes.Buffer{}
	reqBodyGzip := gzip.NewWriter(reqBodyRaw)
	for i, oid := range st.wants {
		if i == 0 {
			oid += " multi_ack_detailed no-done side-band-64k thin-pack ofs-delta deepen-since deepen-not agent=git/2.21.0"
		}
		if _, err := pktline.WriteString(reqBodyGzip, "want "+oid+"\n"); err != nil {
			return err
		}
	}
	if err := pktline.WriteFlush(reqBodyGzip); err != nil {
		return err
	}
	if _, err := pktline.WriteString(reqBodyGzip, "done\n"); err != nil {
		return err
	}
	if err := reqBodyGzip.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", st.URL+"/git-upload-pack", reqBodyRaw)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	for k, v := range map[string]string{
		"User-Agent":       "gitaly-debug",
		"Content-Type":     "application/x-git-upload-pack-request",
		"Accept":           "application/x-git-upload-pack-result",
		"Content-Encoding": "gzip",
	} {
		req.Header.Set(k, v)
	}

	st.post.start = time.Now()
	st.printInteractive("---")
	st.printInteractive("--- POST %v", req.URL)
	st.printInteractive("---")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	st.post.responseHeader = time.Since(st.post.start)
	st.post.httpStatus = resp.StatusCode
	st.printInteractive("response header: %v", resp.Header)

	// Expected response:
	// - "NAK\n"
	// - "<side band byte><pack or progress or error data>
	// - ...
	// - FLUSH
	//

	scanner := pktline.NewScanner(resp.Body)
	payloadSizeHistogram := make(map[int]int)
	seenFlush := false
	for ; scanner.Scan(); st.post.packets++ {
		if seenFlush {
			return errors.New("received extra packet after flush")
		}

		data := pktline.Data(scanner.Bytes())

		if st.post.packets == 0 {
			if !bytes.Equal([]byte("NAK\n"), data) {
				return fmt.Errorf("expected NAK, got %q", data)
			}
			st.post.nak = time.Since(st.post.start)
			continue
		}

		if pktline.IsFlush(scanner.Bytes()) {
			seenFlush = true
			continue
		}

		if len(data) == 0 {
			return errors.New("empty packet in PACK data")
		}

		rawBand := data[0]
		if rawBand < bandMin || rawBand > bandMax {
			return fmt.Errorf("invalid sideband: %d", rawBand)
		}

		band, err := bandToHuman(rawBand)
		if err != nil {
			return err
		}

		info := st.post.multiband[band]
		if info.packets == 0 {
			st.post.multiband[band].firstPacket = time.Since(st.post.start)
		}

		info.packets++

		// Print progress data as-is
		if st.Interactive && band == "progress" {
			if _, err := os.Stdout.Write(data[1:]); err != nil {
				return err
			}
		}

		n := len(data[1:])
		info.size += int64(n)
		payloadSizeHistogram[n]++

		if st.Interactive && st.post.packets%100 == 0 && st.post.packets > 0 && band == "pack" {
			if _, err := fmt.Print("."); err != nil {
				return err
			}
		}
	}

	if st.Interactive {
		// Trailing newline for progress dots.
		if _, err := fmt.Println(""); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	if !seenFlush {
		return errors.New("POST response did not end in flush")
	}

	st.post.responseBody = time.Since(st.post.start)

	for s := range payloadSizeHistogram {
		if s > st.post.largestPayloadSize {
			st.post.largestPayloadSize = s
		}
	}

	return nil
}

func bandToHuman(b byte) (string, error) {
	switch b {
	case 1:
		return "pack", nil
	case 2:
		return "progress", nil
	case 3:
		return "error", nil
	default:
		return "", fmt.Errorf("invalid band %d", b)
	}
}

func (st *Clone) printInteractive(format string, a ...interface{}) error {
	if !st.Interactive {
		return nil
	}

	if _, err := fmt.Println(fmt.Sprintf(format, a...)); err != nil {
		return err
	}

	return nil
}
