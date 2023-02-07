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
				Result    string `json:"result"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type ContainerMetrics struct {
	Aggregation struct {
		Containers struct {
			Buckets []struct {
				Name  string `json:"key"`
				Value struct {
					Value float32 `json:"value"`
				} `json:"avgvalue"`
			} `json:"buckets"`
		} `json:"labels.container"`
	} `json:"aggregations"`
}

// JobSummary store relevant Kube-burner job metadata
type JobSummary struct {
	JobHits struct {
		Hits []struct {
			Results struct {
				UUID      string `json:"uuid"`
				JobConfig struct {
					Iterations    int  `json:"jobIterations"`
					Churn         bool `json:"churn"`
					ChurnPercent  int  `json:"churnPercent"`
					ChurnDuration int  `json:"churnDuration"`
				} `json:"jobConfig"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type Collection struct {
	Metrics       []ContainerMetrics
	UUID          string
	Timestamp     string
	Workload      string
	Version       string
	Platform      string
	Nodes         int
	Iterations    int
	Churn         bool
	ChurnPercent  int
	ChurnDuration int
}

func main() {

	// Namespaces we want to aggregate data from
	ns := []string{"openshift-etcd", "openshift-apiserver", "openshift-ovn-kubernetes"}

	// Metrics we are interested in
	metrics := []string{"containerCPU-Masters"}

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
				 {"query_string": {"query": "ocp_version == %s"}}]}}]}}]}}
		}`, strings.Join(pfilter, ","), strings.Join(wfilter, ","), *fver))
	log.Debug(s)
	search := opensearchapi.SearchRequest{
		Index: []string{"ripsaw-kube-burner*"},
		Body:  s,
	}

	result, err := search.Do(context.Background(), client)
	if err != nil {
		log.Error("Error with search")
		os.Exit(1)
	}
	var d Hits
	json.NewDecoder(result.Body).Decode(&d)
	log.Infof("Found %d results\n", len(d.HitLists.Hit))
	for _, v := range d.HitLists.Hit {
		if v.Results.Result != "Complete" {
			log.Warnf("UUID %s Resulted in %s", v.Results.UUID, v.Results.Result)
			continue
		}
		var c Collection
		if len(v.Results.UUID) < 1 {
			log.Warn("Missing UUID, skipping")
			continue
		}
		log.Infof("Searching UUID %s ", v.Results.UUID)
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
			log.Error("Error with search")
			os.Exit(1)
		}
		var j JobSummary
		json.NewDecoder(r.Body).Decode(&j)
		if len(j.JobHits.Hits) > 0 {
			c.UUID = v.Results.UUID
			c.Iterations = j.JobHits.Hits[0].Results.JobConfig.Iterations
			c.Timestamp = v.Results.Timestamp
			c.Workload = v.Results.Workload
			c.Platform = v.Results.Platform
			c.Version = v.Results.Version
			c.Nodes = v.Results.Nodes
			c.Churn = j.JobHits.Hits[0].Results.JobConfig.Churn
			c.ChurnPercent = j.JobHits.Hits[0].Results.JobConfig.ChurnPercent
			c.ChurnDuration = j.JobHits.Hits[0].Results.JobConfig.ChurnDuration
		} else {
			log.Warnf("Job : %s missing jobSummary", v.Results.UUID)
			continue
		}
		t, _ := strconv.ParseInt(v.Results.Timestamp, 10, 64)
		time := time.UnixMilli(t)
		log.Infof("Test Ran at %s Node count : %d", time.UTC(), v.Results.Nodes)
		for _, n := range ns {
			for _, m := range metrics {
				q := fmt.Sprintf(`"query": {"bool": {"filter": [{"wildcard": {
										"labels.namespace.keyword": "%s"}},{
										"wildcard": {"metricName.keyword": "%s"}}],
										"must": [{"match": {"uuid.keyword": "%s"}}]
										}},"aggs": { "labels.container": { "terms": {"field": "labels.container.keyword",
										"size": 10000 },"aggs": {"avgvalue": {"avg": {"field": "value"}}}}}`, n, m, v.Results.UUID)
				query := strings.NewReader(fmt.Sprintf(`{"size": 10000,%s}`, q))
				js := opensearchapi.SearchRequest{
					Index: []string{"ripsaw-kube-burner*"},
					Body:  query,
				}
				r, err := js.Do(context.Background(), client)
				if err != nil {
					log.Error("Error with search")
					os.Exit(1)
				}
				var cm ContainerMetrics
				json.NewDecoder(r.Body).Decode(&cm)
				if len(cm.Aggregation.Containers.Buckets) > 0 {
					c.Metrics = append(c.Metrics, cm)
				}
			}
		}
		if len(c.Metrics) > 0 {
			col = append(col, c)
		}
	}
	if len(col) > 0 {
		for _, r := range col {
			log.Infof("%-25s | %-15s | %-15s | %-40s | %-15s | %-25s | %-5s | %s", "Version", "Workload", "Platform", "UUID", "Number of Nodes", "Workload Iterations", "Churn", "Date")
			log.Infof("%s", strings.Repeat("-", (25+45+40+15+25+45)))
			t, _ := strconv.ParseInt(r.Timestamp, 10, 64)
			time := time.UnixMilli(t)
			log.Infof("%-25s | %-15s | %-15s | %-40s | %-15d | %-25d | %-5t | %s", r.Version, r.Workload, r.Platform, r.UUID, r.Nodes, r.Iterations, r.Churn, time)
			log.Infof("+ %-45s | %-25s", "Container", "Metric Value")
			log.Infof("+ %s", strings.Repeat("-", (45+25)))
			for _, m := range r.Metrics {
				for _, v := range m.Aggregation.Containers.Buckets {
					log.Infof("+ %-45s | %-25f", v.Name, v.Value.Value)
				}
			}
			log.Infof("%s", strings.Repeat("-", (25+45+40+15+25+45)))
		}
	}
}
