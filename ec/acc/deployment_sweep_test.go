// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package acc

import (
	"strings"
	"sync"

	"github.com/elastic/cloud-sdk-go/pkg/api"
	"github.com/elastic/cloud-sdk-go/pkg/api/deploymentapi"
	"github.com/elastic/cloud-sdk-go/pkg/models"
	"github.com/elastic/cloud-sdk-go/pkg/multierror"
	"github.com/elastic/cloud-sdk-go/pkg/plan"
	"github.com/elastic/cloud-sdk-go/pkg/plan/planutil"
	"github.com/elastic/cloud-sdk-go/pkg/util/ec"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func init() {
	resource.AddTestSweepers("ec_deployment", &resource.Sweeper{
		Name: "ec_deployment",
		F:    testSweepDeployment,
	})
}

func testSweepDeployment(_ string) error {
	client, err := newAPI()
	if err != nil {
		return err
	}

	res, err := deploymentapi.Search(deploymentapi.SearchParams{
		API: client,
		Request: &models.SearchRequest{Query: &models.QueryContainer{
			Prefix: map[string]models.PrefixQuery{
				"name": {Value: ec.String(prefix)},
			},
		}},
	})
	if err != nil {
		return err
	}

	var sweepDeployments []string
	for _, d := range res.Deployments {
		if d.Resources == nil || *d.Metadata.Hidden {
			continue
		}

		if !strings.HasPrefix(*d.Name, prefix) {
			continue
		}

		var sweep bool
		for _, res := range d.Resources.Apm {
			if *res.Info.Status != "stopped" {
				sweep = true
			}
		}
		for _, res := range d.Resources.Elasticsearch {
			if *res.Info.Status != "stopped" {
				sweep = true
			}
		}
		for _, res := range d.Resources.EnterpriseSearch {
			if *res.Info.Status != "stopped" {
				sweep = true
			}
		}
		for _, res := range d.Resources.Kibana {
			if *res.Info.Status != "stopped" {
				sweep = true
			}
		}

		if sweep {
			sweepDeployments = append(sweepDeployments, *d.ID)
		}
	}

	var merr = multierror.NewPrefixed("failed sweeping resources")
	var wg sync.WaitGroup
	for _, dep := range sweepDeployments {
		wg.Add(1)
		go func(id string) {
			if err := shutdownDeployment(client, id, wg.Done); err != nil {
				merr = merr.Append(err)
			}
		}(dep)
	}
	wg.Wait()

	return merr.ErrorOrNil()
}

func shutdownDeployment(c *api.API, dep string, done func()) error {
	defer done()
	_, err := deploymentapi.Shutdown(deploymentapi.ShutdownParams{
		API: c, DeploymentID: dep,
	})
	if err != nil {
		return err
	}

	return planutil.Wait(plan.TrackChangeParams{
		API: c, DeploymentID: dep,
	})
}
