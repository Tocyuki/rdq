// Package connection provides interactive selectors for the RDS cluster,
// Secrets Manager secret, and database name that the rdq CLI feeds to the
// AWS RDS Data API.
package connection

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/term"
)

// ClusterInfo is a flattened view of the RDS DescribeDBClusters output that is
// useful for the picker UI and downstream Data API calls.
type ClusterInfo struct {
	Identifier string
	ARN        string
	Engine     string
	Endpoint   string
}

// ListClusters returns Aurora clusters in the configured region that have the
// RDS Data API enabled (EnableHttpEndpoint == true). Clusters without the
// Data API are filtered out because rdq cannot use them.
func ListClusters(ctx context.Context, cfg aws.Config) ([]ClusterInfo, error) {
	client := rds.NewFromConfig(cfg)
	paginator := rds.NewDescribeDBClustersPaginator(client, &rds.DescribeDBClustersInput{})

	var out []ClusterInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db clusters: %w", err)
		}
		for _, c := range page.DBClusters {
			if !dataAPIEnabled(c) {
				continue
			}
			out = append(out, ClusterInfo{
				Identifier: aws.ToString(c.DBClusterIdentifier),
				ARN:        aws.ToString(c.DBClusterArn),
				Engine:     aws.ToString(c.Engine),
				Endpoint:   aws.ToString(c.Endpoint),
			})
		}
	}
	return out, nil
}

// dataAPIEnabled returns true when the cluster has the RDS Data API turned on.
// The SDK exposes this as the optional *bool field HttpEndpointEnabled.
func dataAPIEnabled(c types.DBCluster) bool {
	return c.HttpEndpointEnabled != nil && *c.HttpEndpointEnabled
}

// SelectCluster launches a fuzzy finder over the given clusters. Fails early
// if there are no clusters, or if stdin is not a TTY.
func SelectCluster(clusters []ClusterInfo) (ClusterInfo, error) {
	if len(clusters) == 0 {
		return ClusterInfo{}, errors.New("no Data API enabled RDS clusters found in this region")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ClusterInfo{}, errors.New("--cluster requires a TTY for interactive selection; pass an ARN explicitly")
	}

	idx, err := fuzzyfinder.Find(clusters, func(i int) string {
		c := clusters[i]
		return fmt.Sprintf("%s [%s] %s", c.Identifier, c.Engine, c.Endpoint)
	}, fuzzyfinder.WithPromptString("RDS cluster> "))
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return ClusterInfo{}, errors.New("cluster selection aborted")
		}
		return ClusterInfo{}, fmt.Errorf("fuzzy finder: %w", err)
	}
	return clusters[idx], nil
}
