package collector

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/container/v1"
)

// KubernetesCollector represents Kubernetes Engine
type KubernetesCollector struct {
	client   *http.Client
	projects []*cloudresourcemanager.Project

	Up    *prometheus.Desc
	Nodes *prometheus.Desc
}

// NewKubernetesCollector creates a new KubernetesCollector
func NewKubernetesCollector(client *http.Client, projects []*cloudresourcemanager.Project) *KubernetesCollector {
	fqName := name("kubernetes_engine")
	labelKeys := []string{
		"name",
		"location",
		"version",
	}
	return &KubernetesCollector{
		client:   client,
		projects: projects,

		Up: prometheus.NewDesc(
			fqName("cluster_up"),
			"1 if the cluster is running, 0 otherwise",
			labelKeys,
			nil,
		),
		Nodes: prometheus.NewDesc(
			fqName("cluster_nodes"),
			"Number of nodes currently in the cluster",
			labelKeys,
			nil,
		),
	}
}

// Collect implements Prometheus' Collector interface and is used to collect metrics
func (c *KubernetesCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	containerService, err := container.New(c.client)
	if err != nil {
		log.Println(err)
	}

	// Enumerate all of the projects
	var wg sync.WaitGroup
	for _, p := range c.projects {
		wg.Add(1)
		go func(p *cloudresourcemanager.Project) {
			defer wg.Done()
			log.Printf("[KubernetesCollector:go] Project: %s", p.ProjectId)
			parent := fmt.Sprintf("projects/%s/locations/-", p.ProjectId)
			resp, err := containerService.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
			if err != nil {
				log.Println(err)
				return
			}

			for _, cluster := range resp.Clusters {
				log.Printf("[KubernetesCollector] cluster: %s", cluster.Name)
				ch <- prometheus.MustNewConstMetric(
					c.Up,
					prometheus.CounterValue,
					func(c *container.Cluster) (result float64) {
						if c.Status == "RUNNING" {
							result = 1.0
						}
						return result
					}(cluster),
					[]string{
						cluster.Name,
						cluster.Location,
						cluster.CurrentNodeVersion,
					}...,
				)
				ch <- prometheus.MustNewConstMetric(
					c.Nodes,
					prometheus.GaugeValue,
					float64(cluster.CurrentNodeCount),
					[]string{
						cluster.Name,
						cluster.Location,
						cluster.CurrentNodeVersion,
					}...,
				)
			}
		}(p)
	}
	wg.Wait()

}

// Describe implements Prometheus' Collector interface and is used to describe metrics
func (c *KubernetesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.Up
	ch <- c.Nodes
}
