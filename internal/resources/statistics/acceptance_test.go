package statistics_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

type statisticEntry struct {
	Name       string            `json:"name"`
	Value      string            `json:"value"`
	Reserved   bool              `json:"reserved"`
	Modifiable bool              `json:"modifiable"`
	Audit      map[string]string `json:"audit"`
}

func statisticAudit() map[string]string {
	return map[string]string{
		"creator":    "user1",
		"createTime": "2023-10-01T12:00:00Z",
	}
}

// fakeStatisticsServer stores the custom statistics the resources write and
// reports one reserved statistic alongside, as a real Gravitino server does for
// a lakehouse table.
type fakeStatisticsServer struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *fakeStatisticsServer) entries() []statisticEntry {
	entries := []statisticEntry{{
		Name:     "row_count",
		Value:    "1000",
		Reserved: true,
		Audit:    statisticAudit(),
	}}

	names := make([]string, 0, len(s.values))
	for name := range s.values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entries = append(entries, statisticEntry{
			Name:       name,
			Value:      s.values[name],
			Modifiable: true,
			Audit:      statisticAudit(),
		})
	}
	return entries
}

// TestAccStatisticsResource drives create, refresh, update, import and destroy
// of gravitino_statistics through Terraform Core.
func TestAccStatisticsResource(t *testing.T) {
	fake := &fakeStatisticsServer{values: map[string]string{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)

		case r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.orders/statistics":
			fake.mu.Lock()
			defer fake.mu.Unlock()

			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"code":       0,
					"statistics": fake.entries(),
				})
			case http.MethodPut:
				var body struct {
					Updates map[string]string `json:"updates"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				for name, value := range body.Updates {
					fake.values[name] = value
				}
				fmt.Fprint(w, `{"code":0}`)
			case http.MethodPost:
				var body struct {
					Names []string `json:"names"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				for _, name := range body.Names {
					delete(fake.values, name)
				}
				fmt.Fprint(w, `{"code":0,"dropped":true}`)
			default:
				http.NotFound(w, r)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_statistics" "this" {
  metalake         = "ml"
  object_type      = "TABLE"
  object_full_name = "sales.orders"

  statistics = {
    "custom-k1" = "v1"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_statistics.this", "id", "ml.TABLE.sales.orders"),
					resource.TestCheckResourceAttr("gravitino_statistics.this", "statistics.%", "1"),
					resource.TestCheckResourceAttr("gravitino_statistics.this", "statistics.custom-k1", "v1"),
				),
			},
			{
				ResourceName:      "gravitino_statistics.this",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "gravitino_statistics" "this" {
  metalake         = "ml"
  object_type      = "TABLE"
  object_full_name = "sales.orders"

  statistics = {
    "custom-k2" = "v2"
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_statistics.this", "statistics.%", "1"),
					resource.TestCheckResourceAttr("gravitino_statistics.this", "statistics.custom-k2", "v2"),
					resource.TestCheckNoResourceAttr("gravitino_statistics.this", "statistics.custom-k1"),
				),
			},
		},
		CheckDestroy: func(*terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.values) != 0 {
				return fmt.Errorf("destroy left statistics on the server: %v", fake.values)
			}
			return nil
		},
	})
}

// fakePartitionStatisticsServer stores the partition statistics the resource
// writes.
type fakePartitionStatisticsServer struct {
	mu         sync.Mutex
	partitions map[string]map[string]string
}

func (s *fakePartitionStatisticsServer) response() map[string]interface{} {
	names := make([]string, 0, len(s.partitions))
	for name := range s.partitions {
		names = append(names, name)
	}
	sort.Strings(names)

	partitions := make([]interface{}, 0, len(names))
	for _, name := range names {
		statisticNames := make([]string, 0, len(s.partitions[name]))
		for statistic := range s.partitions[name] {
			statisticNames = append(statisticNames, statistic)
		}
		sort.Strings(statisticNames)

		statistics := make([]statisticEntry, 0, len(statisticNames))
		for _, statistic := range statisticNames {
			statistics = append(statistics, statisticEntry{
				Name:       statistic,
				Value:      s.partitions[name][statistic],
				Modifiable: true,
				Audit:      statisticAudit(),
			})
		}
		partitions = append(partitions, map[string]interface{}{
			"partitionName": name,
			"statistics":    statistics,
		})
	}

	return map[string]interface{}{"code": 0, "partitionStatistics": partitions}
}

// TestAccPartitionStatisticsResource drives create, refresh, update and destroy
// of gravitino_partition_statistics through Terraform Core. It also proves that
// the nested map schema (partitions -> statistics) round-trips.
func TestAccPartitionStatisticsResource(t *testing.T) {
	fake := &fakePartitionStatisticsServer{partitions: map[string]map[string]string{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			fmt.Fprint(w, `{"code":0,"version":{"version":"1.3.1","compileDate":"2026-01-01","gitCommit":"abc"}}`)

		case r.URL.Path == "/api/metalakes/ml/objects/TABLE/sales.public.orders/statistics/partitions":
			fake.mu.Lock()
			defer fake.mu.Unlock()

			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(fake.response())
			case http.MethodPut:
				var body struct {
					Updates []struct {
						PartitionName string            `json:"partitionName"`
						Statistics    map[string]string `json:"statistics"`
					} `json:"updates"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				for _, update := range body.Updates {
					if fake.partitions[update.PartitionName] == nil {
						fake.partitions[update.PartitionName] = map[string]string{}
					}
					for name, value := range update.Statistics {
						fake.partitions[update.PartitionName][name] = value
					}
				}
				fmt.Fprint(w, `{"code":0}`)
			case http.MethodPost:
				var body struct {
					Drops []struct {
						PartitionName  string   `json:"partitionName"`
						StatisticNames []string `json:"statisticNames"`
					} `json:"drops"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				for _, drop := range body.Drops {
					for _, name := range drop.StatisticNames {
						delete(fake.partitions[drop.PartitionName], name)
					}
					if len(fake.partitions[drop.PartitionName]) == 0 {
						delete(fake.partitions, drop.PartitionName)
					}
				}
				fmt.Fprint(w, `{"code":0,"dropped":true}`)
			default:
				http.NotFound(w, r)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_partition_statistics" "this" {
  metalake = "ml"
  catalog  = "sales"
  schema   = "public"
  table    = "orders"

  partitions = {
    p0 = {
      "custom-row_count" = "1000"
    }
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "id", "ml.sales.public.orders"),
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "partitions.%", "1"),
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "partitions.p0.custom-row_count", "1000"),
				),
			},
			{
				ResourceName:      "gravitino_partition_statistics.this",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "gravitino_partition_statistics" "this" {
  metalake = "ml"
  catalog  = "sales"
  schema   = "public"
  table    = "orders"

  partitions = {
    p0 = {
      "custom-row_count" = "2000"
    }
    p1 = {
      "custom-file_size" = "1GB"
    }
  }
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "partitions.%", "2"),
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "partitions.p0.custom-row_count", "2000"),
					resource.TestCheckResourceAttr("gravitino_partition_statistics.this", "partitions.p1.custom-file_size", "1GB"),
				),
			},
		},
		CheckDestroy: func(*terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.partitions) != 0 {
				return fmt.Errorf("destroy left partition statistics on the server: %v", fake.partitions)
			}
			return nil
		},
	})
}
