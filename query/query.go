package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

// Hits struct contains the results from OpenSearch
type Hits struct {
	HitLists struct {
		Hit []struct {
			Results struct {
				UUID     string `json:"uuid"`
				Workload string `json:"workload"`
				Version  string `json:"ocp_version"`
				Platform string `json:"platform"`
				Nodes    int    `json:"total_nodes"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func main() {
	fplat := flag.String("platforms", "AWS", "Platforms to filter on, pass as AWS,ROSA")
	fwork := flag.String("workloads", "cluster-density", "Workloads to filter on, pass as cluster-density,node-density")
	flag.Parse()
	server := os.Getenv("ES_URL")
	if len(server) < 1 {
		fmt.Println("ES_URL env var cannot be empty")
		os.Exit(1)
	}
	config := opensearch.Config{
		Addresses: []string{server},
	}
	client, err := opensearch.NewClient(config)
	if err != nil {
		fmt.Println("Unable to connect ES")
		os.Exit(1)
	}
	fmt.Printf("Connected to : %s\n", config.Addresses)
	plat := strings.Split(*fplat, ",")
	work := strings.Split(*fwork, ",")
	pfilter := []string{}
	wfilter := []string{}
	for _, p := range plat {
		f := fmt.Sprintf(`{
		"bool": {
		  "should": [
			{
			  "match_phrase": {
				"platform.keyword": "%s"
			  }
			}
		  ],
		  "minimum_should_match": 1
		}
	  }`, p)
		pfilter = append(pfilter, f)
	}
	for _, w := range work {
		f := fmt.Sprintf(`{
			"bool": {
			  "should": [
				{
				  "match_phrase": {
					"benchmark.keyword": "%s"
				  }
				}
			  ],
			  "minimum_should_match": 1
			}
		  }`, w)
		wfilter = append(wfilter, f)
	}

	s := strings.NewReader(fmt.Sprintf(`{
		"size": 10000,
		"query": {"bool": {	"must": [],	"filter": [ { "bool": { "filter": [ { "bool": { "should": [ %s ],"minimum_should_match": 1 } },
				 { "bool": {	"filter": [ { "bool": { "should": [ %s ], "minimum_should_match": 1 }},
				 {"query_string": {"query": "ocp_version \\=\\= 4.1*"}}]}}]}}]}}
		}`, strings.Join(pfilter, ","), strings.Join(wfilter, ",")))

	search := opensearchapi.SearchRequest{
		Index: []string{"ripsaw-kube-burner*"},
		Body:  s,
	}

	result, err := search.Do(context.Background(), client)
	if err != nil {
		fmt.Println("Error with search")
		os.Exit(1)
	}
	var d Hits
	json.NewDecoder(result.Body).Decode(&d)
	fmt.Printf("Found %d results\n", len(d.HitLists.Hit))
}
