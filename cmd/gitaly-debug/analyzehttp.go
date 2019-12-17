package main

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitaly/internal/git/stats"
)

type metric struct {
	key   string
	value interface{}
}

func analyzeHTTPClone(cloneURL string) {
	st := &stats.Clone{
		URL:         cloneURL,
		Interactive: true,
	}

	noError(st.Perform(context.Background()))

	fmt.Println("\n--- GET metrics:")
	for _, entry := range []metric{
		{"response header time", st.Get.ResponseHeader()},
		{"first Git packet", st.Get.FirstGitPacket()},
		{"response body time", st.Get.ResponseBody()},
		{"payload size", st.Get.PayloadSize()},
		{"Git packets received", st.Get.Packets()},
		{"refs advertised", st.Get.RefsAdvertised()},
		{"wanted refs", st.RefsWanted()},
	} {
		fmt.Printf("%s %v\n", entry.key, entry.value)
	}

	fmt.Println("\n--- POST metrics:")
	for _, entry := range []metric{
		{"response header time", st.Post.ResponseHeader()},
		{"time to server NAK", st.Post.NAK()},
		{"response body time", st.Post.ResponseBody()},
		{"largest single Git packet payload", st.Post.LargestPayloadSize()},
		{"Git packets received", st.Post.Packets()},
	} {
		fmt.Printf("%s %v\n", entry.key, entry.value)
	}

	for _, band := range stats.Bands() {
		numPackets := st.Post.BandPackets(band)
		if numPackets == 0 {
			continue
		}

		fmt.Printf("\n--- POST %s band\n", band)
		for _, entry := range []metric{
			{"time to first packet", st.Post.BandFirstPacket(band)},
			{"packets", numPackets},
			{"total payload size", st.Post.BandPayloadSize(band)},
		} {
			fmt.Printf("%s %v\n", entry.key, entry.value)
		}
	}
}
