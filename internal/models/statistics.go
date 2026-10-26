package models

import (
	"bytes"
	"encoding/json"
)

const (
	// StatisticCustomPrefix is the only statistic name prefix Gravitino accepts
	// in updateStatistics/updatePartitionStatistics ("Currently we only support
	// custom statistics.", StatisticOperations.validateStatisticName).
	StatisticCustomPrefix = "custom-"

	// StatisticMaxNameLength is Statistic.MAX_NAME_LENGTH.
	StatisticMaxNameLength = 128
)

// Statistic is one entry of the Statistic schema in statistics.yaml.
//
// Value is untyped in the spec (`additionalProperties: {}`): a JSON string,
// number, boolean, array or object. The raw JSON is kept so a response is never
// re-encoded into a different type; callers render it with ValueString.
type Statistic struct {
	Name       string          `json:"name"`
	Value      json.RawMessage `json:"value,omitempty"`
	Reserved   bool            `json:"reserved"`
	Modifiable bool            `json:"modifiable"`
	Audit      *Audit          `json:"audit,omitempty"`
}

// ValueString renders the statistic value as a Terraform string: a JSON string
// contributes its text, every other JSON scalar, array or object its JSON
// encoding. The second result is false when the statistic has no value (the
// spec makes `value` optional) or the value is JSON null.
//
// This is the provider's typing decision for statistics: the API's value is
// untyped, the Terraform surface is a map of strings, so every value crosses
// the boundary as a string.
func (s *Statistic) ValueString() (string, bool) {
	raw := bytes.TrimSpace(s.Value)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}

	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text, true
		}
	}

	return string(raw), true
}

type StatisticsResponse struct {
	Code       int         `json:"code"`
	Statistics []Statistic `json:"statistics"`
}

type PartitionStatistics struct {
	PartitionName string      `json:"partitionName"`
	Statistics    []Statistic `json:"statistics,omitempty"`
}

type PartitionStatisticsResponse struct {
	Code       int                   `json:"code"`
	Statistics []PartitionStatistics `json:"partitionStatistics,omitempty"`
}

// StatisticsUpdateRequest is the body of updateStatistics (PUT .../statistics).
// Values are sent as JSON strings: the spec's additionalProperties is untyped,
// but the Terraform schema is a map of strings.
type StatisticsUpdateRequest struct {
	Updates map[string]string `json:"updates"`
}

// StatisticsDropRequest is the body of dropStatistics (POST .../statistics).
type StatisticsDropRequest struct {
	Names []string `json:"names"`
}

// PartitionStatisticsUpdateRequest is the body of updatePartitionStatistics
// (PUT .../statistics/partitions).
type PartitionStatisticsUpdateRequest struct {
	Updates []PartitionStatisticsUpdate `json:"updates"`
}

type PartitionStatisticsUpdate struct {
	PartitionName string            `json:"partitionName"`
	Statistics    map[string]string `json:"statistics"`
}

// PartitionStatisticsDropRequest is the body of dropPartitionStatistics
// (POST .../statistics/partitions).
type PartitionStatisticsDropRequest struct {
	Drops []PartitionStatisticsDrop `json:"drops"`
}

type PartitionStatisticsDrop struct {
	PartitionName  string   `json:"partitionName"`
	StatisticNames []string `json:"statisticNames"`
}
