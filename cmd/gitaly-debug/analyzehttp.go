package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlab.com/gitlab-org/gitaly/internal/git/pktline"
)

func analyzeHTTPClone(cloneURL string, formatJSON bool) {
	stats := &cloneStats{
		URL:  cloneURL,
		json: formatJSON,
	}
	stats.doGet()
	stats.doPost()
}

type cloneStats struct {
	URL   string
	json  bool
	wants []string
	get
	post
}

type get struct {
	start              time.Time
	responseHeaderTime time.Duration
	firstPacketTime    time.Duration
	totalTime          time.Duration
	gzip               bool
	status             int
	payloadSize        int64
	packets            int
	refs               int
}

func (st *cloneStats) doGet() {
	req, err := http.NewRequest("GET", st.URL+"/info/refs?service=git-upload-pack", nil)
	noError(err)

	for k, v := range map[string]string{
		"User-Agent":      "gitaly-debug",
		"Accept":          "*/*",
		"Accept-Encoding": "deflate, gzip",
		"Pragma":          "no-cache",
	} {
		req.Header.Set(k, v)
	}

	st.get.start = time.Now()
	st.msg("---")
	st.msg("--- GET %v", req.URL)
	st.msg("---")
	resp, err := http.DefaultClient.Do(req)
	noError(err)

	st.get.responseHeaderTime = time.Since(st.get.start)
	st.get.status = resp.StatusCode
	defer resp.Body.Close()

	st.msg("response after %v", st.get.responseHeaderTime)
	st.msg("response header: %v", resp.Header)
	st.msg("HTTP status code %d", st.get.status)

	body := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		st.gzip = true
		body, err = gzip.NewReader(body)
		noError(err)
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
			fatal("received packet after flush")
		}

		data := string(pktline.Data(scanner.Bytes()))
		st.get.payloadSize += int64(len(data))
		switch st.get.packets {
		case 0:
			st.firstPacketTime = time.Since(st.get.start)
			st.msg("first packet %v", st.firstPacketTime)
			if data != "# service=git-upload-pack\n" {
				fatal(fmt.Errorf("unexpected header %q", data))
			}
		case 1:
			if !pktline.IsFlush(scanner.Bytes()) {
				fatal("missing flush after service announcement")
			}
		default:
			if st.get.packets == 2 && !strings.Contains(data, " side-band-64k") {
				fatal(fmt.Errorf("missing side-band-64k capability in %q", data))
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
	noError(scanner.Err())
	if !seenFlush {
		fatal("missing flush in response")
	}

	st.totalTime = time.Since(st.get.start)

	st.msg("received %d packets", st.get.packets)
	st.msg("done in %v", st.totalTime)
	st.msg("payload data: %d bytes", st.get.payloadSize)
	st.msg("received %d refs, selected %d wants", st.get.refs, len(st.wants))
}

type post struct {
	start              time.Time
	responseHeaderTime time.Duration
	nakTime            time.Duration
	multiband          map[string]*bandInfo
	status             int
	packets            int
	largestPayloadSize int
}

type bandInfo struct {
	first   time.Duration
	size    int64
	packets int
}

const (
	bandMin = 1
	bandMax = 3
)

func (st *cloneStats) doPost() {
	st.multiband = make(map[string]*bandInfo)
	for i := byte(bandMin); i < bandMax; i++ {
		st.multiband[bandToHuman(i)] = &bandInfo{}
	}

	reqBodyRaw := &bytes.Buffer{}
	reqBodyGzip := gzip.NewWriter(reqBodyRaw)
	for i, oid := range st.wants {
		if i == 0 {
			oid += " multi_ack_detailed no-done side-band-64k thin-pack ofs-delta deepen-since deepen-not agent=git/2.21.0"
		}
		_, err := pktline.WriteString(reqBodyGzip, "want "+oid+"\n")
		noError(err)
	}
	noError(pktline.WriteFlush(reqBodyGzip))
	_, err := pktline.WriteString(reqBodyGzip, "done\n")
	noError(err)
	noError(reqBodyGzip.Close())

	req, err := http.NewRequest("POST", st.URL+"/git-upload-pack", reqBodyRaw)
	noError(err)

	for k, v := range map[string]string{
		"User-Agent":       "gitaly-debug",
		"Content-Type":     "application/x-git-upload-pack-request",
		"Accept":           "application/x-git-upload-pack-result",
		"Content-Encoding": "gzip",
	} {
		req.Header.Set(k, v)
	}

	st.post.start = time.Now()
	st.msg("---")
	st.msg("--- POST %v", req.URL)
	st.msg("---")

	resp, err := http.DefaultClient.Do(req)
	noError(err)
	defer resp.Body.Close()

	st.post.responseHeaderTime = time.Since(st.post.start)
	st.post.status = resp.StatusCode

	st.msg("response after %v", st.post.responseHeaderTime)
	st.msg("response header: %v", resp.Header)
	st.msg("HTTP status code %d", st.post.status)

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
			fatal("received extra packet after flush")
		}

		data := pktline.Data(scanner.Bytes())

		if st.post.packets == 0 {
			if !bytes.Equal([]byte("NAK\n"), data) {
				fatal(fmt.Errorf("expected NAK, got %q", data))
			}
			st.post.nakTime = time.Since(st.post.start)
			st.msg("received NAK after %v", st.post.nakTime)
			continue
		}

		if pktline.IsFlush(scanner.Bytes()) {
			seenFlush = true
			continue
		}

		if len(data) == 0 {
			fatal("empty packet in PACK data")
		}

		rawBand := data[0]
		if rawBand < bandMin || rawBand > bandMax {
			fatal(fmt.Errorf("invalid sideband: %d", rawBand))
		}

		band := bandToHuman(rawBand)
		info := st.post.multiband[band]
		if info.packets == 0 {
			info.first = time.Since(st.post.start)
			st.msg("received first %s packet after %v", band, info.first)
		}

		info.packets++

		// Print progress data as-is
		if !st.json && band == "progress" {
			_, err := os.Stdout.Write(data[1:])
			noError(err)
		}

		n := len(data[1:])
		info.size += int64(n)
		payloadSizeHistogram[n]++

		if !st.json && st.post.packets%100 == 0 && st.post.packets > 0 && band == "pack" {
			fmt.Printf(".")
		}
	}

	if !st.json {
		fmt.Println("") // Trailing newline for progress dots.
	}

	noError(scanner.Err())
	if !seenFlush {
		fatal("POST response did not end in flush")
	}

	st.msg("received %d packets", st.post.packets)
	st.msg("done in %v", time.Since(st.post.start))

	for band, info := range st.post.multiband {
		st.msg("%8s band: %10d payload bytes, %6d packets", band, info.size, info.packets)
	}
	st.msg("packet payload size histogram: %v", payloadSizeHistogram)

	for s := range payloadSizeHistogram {
		if s > st.post.largestPayloadSize {
			st.post.largestPayloadSize = s
		}
	}

	if st.json {
		fmt.Printf("%+v\n", st.get)
		fmt.Printf("%+v\n", st.post)
	}
}

func bandToHuman(b byte) string {
	switch b {
	case 1:
		return "pack"
	case 2:
		return "progress"
	case 3:
		return "error"
	default:
		fatal(fmt.Errorf("invalid band %d", b))
		return "" // never reached
	}
}

func (st *cloneStats) msg(format string, a ...interface{}) {
	if !st.json {
		msg(format, a...)
	}
}
