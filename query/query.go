package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	log "tinker/query/logging"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

// Hits struct contains the results from OpenSearch
type Hits struct {
	HitLists struct {
		Hit []struct {
			Results struct {
				UUID      string `json:"uuid"`
				Timestamp string `json:"timestamp"`
				Workload  string `json:"workload"`
				Version   string `json:"ocp_version"`
				Platform  string `json:"platform"`
				Nodes     int    `json:"total_nodes"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type JobSummary struct {
	JobHits struct {
		Hits []struct {
			Results struct {
				UUID      string `json:"uuid"`
				JobConfig struct {
					Iterations    int  `json:"jobIterations"`
					Chrun         bool `json:"churn"`
					ChurnPercent  int  `json:"churnPercent"`
					ChurnDuration int  `json:"churnDuration"`
				} `json:"jobConfig"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type Collection struct {
	UUID          string
	Timestamp     string
	Workload      string
	Version       string
	Platform      string
	Nodes         int
	Iterations    int
	Chrun         bool
	ChurnPercent  int
	ChurnDuration int
}

func main() {
	var col []Collection
	fplat := flag.String("platforms", "AWS", "Platforms to filter on, pass as AWS,ROSA")
	fwork := flag.String("workloads", "cluster-density", "Workloads to filter on, pass as cluster-density,node-density")
	fver := flag.String("version", "4.12*", "Version to search on, use * to wildcard")
	flag.Parse()
	log.Infof("Platform Filter : %s", *fplat)
	log.Infof("Workload Filter : %s", *fwork)
	log.Infof("Version Query : %s", *fver)
	server := os.Getenv("ES_URL")
	if len(server) < 1 {
		log.Error("ES_URL env var cannot be empty")
		os.Exit(1)
	}
	config := opensearch.Config{
		Addresses: []string{server},
	}
	client, err := opensearch.NewClient(config)
	if err != nil {
		log.Error("Unable to connect ES")
		os.Exit(1)
	}
	log.Infof("Connected to : %s\n", config.Addresses)
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
				 {"query_string": {"query": "ocp_version \\=\\= %s"}}]}}]}}]}}
		}`, strings.Join(pfilter, ","), strings.Join(wfilter, ","), *fver))
	log.Debug(s)
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
	for _, v := range d.HitLists.Hit {
		var c Collection
		jsum := strings.NewReader(fmt.Sprintf(`{"size": 10000,
			"query": { "bool": { "must": [], "filter": [ { "bool": {"should": [ {"match": { "metricName": "jobSummary" } }],"minimum_should_match": 1}},
			{"match_phrase": {"uuid": "%s"}}]}}}`, v.Results.UUID))
		log.Debug(jsum)
		js := opensearchapi.SearchRequest{
			Index: []string{"ripsaw-kube-burner*"},
			Body:  jsum,
		}
		r, err := js.Do(context.Background(), client)
		if err != nil {
			fmt.Println("Error with search")
			os.Exit(1)
		}
		var j JobSummary
		json.NewDecoder(r.Body).Decode(&j)
		if len(j.JobHits.Hits) > 0 {
			c.Iterations = j.JobHits.Hits[0].Results.JobConfig.Iterations
			c.UUID = v.Results.UUID
			c.Timestamp = v.Results.Timestamp
			c.Workload = v.Results.Workload
		} else {
			log.Warnf("Job : %s missing jobSummary", v.Results.UUID)
		}
		col = append(col, c)
		t, _ := strconv.ParseInt(v.Results.Timestamp, 10, 64)
		time := time.UnixMilli(t)
		log.Infof("Test Ran at %s Node count : %d", time.UTC(), v.Results.Nodes)
	}
	log.Info(col)
}
