// Command praefect provides a subcommand "dial-nodes". The subcommand
// checks if Praefect can successfully dial to all Gitaly nodes specified in
// the config file and successfully ping the health checker:
//
//     praefect dial-nodes -config PATH_TO_CONFIG
//
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync"
	"time"

	"gitlab.com/gitlab-org/gitaly/client"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type nodePingResult struct {
	address   string
	storages  map[string]struct{} // set of storages this node hosts
	vStorages map[string]struct{} // set of virtual storages node belongs to
	token     string              // auth token
	err       error               // any error during dial/ping
}

func setToSortedList(m map[string]struct{}) []string {
	var list []string
	for k := range m {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

func (npr nodePingResult) String() string {
	return fmt.Sprintf(
		"Node on address %s:\n"+
			"\t-Part of virtual storages %v\n"+
			"\t-Hosts storage locations %v\n",
		npr.address,
		setToSortedList(npr.vStorages),
		setToSortedList(npr.storages),
	)

}

func dialNodes(conf config.Config) int {
	nodes := map[string]nodePingResult{} // key is address

	// flatten nodes between virtual storages
	for _, vs := range conf.VirtualStorages {
		for _, node := range vs.Nodes {
			n := nodes[node.Address]
			n.address = node.Address
			n.storages[node.Storage] = struct{}{}
			n.vStorages[vs.Name] = struct{}{}
			n.token = node.Token
		}
	}

	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		<-interrupt
		os.Exit(130) // indicates program was interrupted
	}()

	var wg sync.WaitGroup
	for _, n := range nodes {
		n := n // rescope

		log.Print(n)

		wg.Add(1)
		go func() {
			defer wg.Done()
			dialPingNode(&n)
		}()
	}
	wg.Wait()

	exitCode := 0
	for _, n := range nodes {
		if n.err != nil {
			exitCode = 1
		}
	}

	return exitCode
}

func dialPingNode(npr *nodePingResult) {
	log.Printf("Dialing %s", npr.address)
	cc, err := client.Dial(npr.address, []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(30 * time.Second),
	})
	if err != nil {
		npr.err = err
		log.Printf("ERROR: unable to dial %s: %v\n", npr.address, err)
		return
	}
	log.Printf("PROGRESS: sucessfully dialed %s\n", npr.address)

	hClient := grpc_health_v1.NewHealthClient(cc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := hClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		npr.err = err
		log.Printf("ERROR: unable to ping health check for %s: %v\n",
			npr.address, err)
		return
	}

	if status := resp.GetStatus(); status != grpc_health_v1.HealthCheckResponse_SERVING {
		npr.err = fmt.Errorf("health check for %s did not report healthy, instead reported: %s", npr.address, status.String())
		log.Printf("ERROR: %v\n", npr.err)
	}

	log.Printf("SUCCESS: able to dial and ping health check for %s\n", npr.address)
	return
}
