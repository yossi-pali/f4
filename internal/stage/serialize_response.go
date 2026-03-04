package stage

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/event"
	"github.com/12go/f4/internal/pipeline"
)

// SearchResponse is the final output of Stage 9 (the entire pipeline).
type SearchResponse struct {
	Trips          []domain.TripResult        `json:"trips"`
	Recheck        []string                   `json:"recheck"`
	Stations       map[int]domain.Station     `json:"stations,omitempty"`
	Operators      map[int]domain.Operator    `json:"operators,omitempty"`
	Classes        map[int]domain.VehicleClass `json:"classes,omitempty"`
	ProvinceName   string                     `json:"provinceName,omitempty"`
	StageTimes     map[string]time.Duration   `json:"-"`
}

// SerializeResponseStage builds the final API response and emits events.
type SerializeResponseStage struct {
	publisher      event.Publisher
	recheckBaseURL string
}

func NewSerializeResponseStage(publisher event.Publisher, recheckBaseURL string) *SerializeResponseStage {
	return &SerializeResponseStage{
		publisher:      publisher,
		recheckBaseURL: recheckBaseURL,
	}
}

func (s *SerializeResponseStage) Name() string { return "serialize_response" }

func (s *SerializeResponseStage) Execute(ctx context.Context, in FinalResults) (SearchResponse, error) {
	resp := SearchResponse{
		Trips:        in.Trips,
		Stations:     in.Stations,
		Operators:    in.Operators,
		Classes:      in.Classes,
		ProvinceName: in.ToProvinceName,
	}

	if resp.Trips == nil {
		resp.Trips = []domain.TripResult{}
	}

	// Build recheck URLs (one per integration chunk group, matching PHP Rechecker)
	if len(in.RecheckGroups) > 0 || len(in.PackRecheckGroups) > 0 {
		resp.Recheck = s.buildRecheckURLs(in)
		// PHP Rechecker appends pack recheck URLs after regular recheck URLs
		resp.Recheck = append(resp.Recheck, s.buildPackManualRecheckURLs(in)...)
	}
	if resp.Recheck == nil {
		resp.Recheck = []string{}
	}

	// Collect stage times from pipeline context
	if pc := pipeline.FromContext(ctx); pc != nil {
		resp.StageTimes = pc.StageTimes()
	}

	// Emit events (fire-and-forget, errors are non-fatal)
	s.emitEvents(ctx, in)

	return resp, nil
}

// buildRecheckURLs generates one URL per RecheckGroup, matching PHP Rechecker::getRecheckUrls.
// URL is built manually (not via url.Values) to match PHP's exact parameter order and encoding:
// commas in f/t are NOT percent-encoded; search_url IS percent-encoded (PHP uses http_build_query).
func (s *SerializeResponseStage) buildRecheckURLs(in FinalResults) []string {
	if s.recheckBaseURL == "" {
		return nil
	}

	dateStr := in.Filter.Date.Format("2006-01-02")
	urls := make([]string, 0, len(in.RecheckGroups))

	for _, g := range in.RecheckGroups {
		var b strings.Builder
		b.WriteString(s.recheckBaseURL)
		b.WriteString("?l=")
		b.WriteString(in.Filter.Lang)
		b.WriteString("&f=")
		b.WriteString(intsToString(g.FromStationIDs))
		b.WriteString("&t=")
		b.WriteString(intsToString(g.ToStationIDs))
		b.WriteString("&d=")
		b.WriteString(dateStr)
		b.WriteString("&sa=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsAdult))
		b.WriteString("&sc=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsChild))
		b.WriteString("&si=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsInfant))
		b.WriteString("&a=")
		b.WriteString(strconv.Itoa(in.Filter.AgentID))
		if in.Filter.SearchURL != "" {
			b.WriteString("&search_url=")
			b.WriteString(url.QueryEscape(in.Filter.SearchURL))
		}
		b.WriteString("&visitorId=")
		b.WriteString(in.Filter.VisitorID)

		urls = append(urls, b.String())
	}

	return urls
}

// buildPackManualRecheckURLs generates /searchpm URLs for manual pack recheck groups.
// PHP Rechecker: foreach ($recheck->manualPacks as $recheckGroup) { ... }
// URL format: /searchpm?t=headTripKey1 tripKey1 date1,headTripKey2 tripKey2 date2&l=...&d=...&...
func (s *SerializeResponseStage) buildPackManualRecheckURLs(in FinalResults) []string {
	if s.recheckBaseURL == "" || len(in.PackRecheckGroups) == 0 {
		return nil
	}

	// Derive /searchpm base URL from /searchr base URL
	packBaseURL := strings.Replace(s.recheckBaseURL, "/searchr", "/searchpm", 1)
	dateStr := in.Filter.Date.Format("2006-01-02")

	urls := make([]string, 0, len(in.PackRecheckGroups))
	for _, g := range in.PackRecheckGroups {
		if len(g.Entries) == 0 {
			continue
		}

		// Build t= parameter: "headTripKey tripKey date" joined by commas
		tripParts := make([]string, 0, len(g.Entries))
		for _, e := range g.Entries {
			tripParts = append(tripParts, e.HeadTripKey+" "+e.TripKey+" "+e.Date)
		}

		var b strings.Builder
		b.WriteString(packBaseURL)
		b.WriteString("?t=")
		b.WriteString(strings.Join(tripParts, ","))
		b.WriteString("&l=")
		b.WriteString(in.Filter.Lang)
		b.WriteString("&d=")
		b.WriteString(dateStr)
		b.WriteString("&sa=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsAdult))
		b.WriteString("&sc=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsChild))
		b.WriteString("&si=")
		b.WriteString(strconv.Itoa(in.Filter.SeatsInfant))
		b.WriteString("&a=")
		b.WriteString(strconv.Itoa(in.Filter.AgentID))
		if in.Filter.SearchURL != "" {
			b.WriteString("&search_url=")
			b.WriteString(in.Filter.SearchURL)
		}
		b.WriteString("&visitorId=")
		b.WriteString(in.Filter.VisitorID)

		urls = append(urls, b.String())
	}

	return urls
}

func (s *SerializeResponseStage) emitEvents(ctx context.Context, in FinalResults) {
	dateStr := in.Filter.Date.Format("2006-01-02")

	// search.completed event
	_ = s.publisher.Publish(ctx, domain.TopicSearchCompleted, domain.SearchCompletedEvent{
		FromPlaceID: in.Filter.FromPlaceID,
		ToPlaceID:   in.Filter.ToPlaceID,
		Date:        dateStr,
		ResultCount: len(in.Trips),
		AgentID:     in.Filter.AgentID,
		Timestamp:   time.Now(),
	})

	// search.needs_recheck event
	if len(in.RecheckTripKeys) > 0 {
		_ = s.publisher.Publish(ctx, domain.TopicSearchNeedsRecheck, domain.SearchNeedsRecheckEvent{
			TripKeys:       in.RecheckTripKeys,
			FromStationIDs: in.Filter.FromStationIDs,
			ToStationIDs:   in.Filter.ToStationIDs,
			Date:           dateStr,
			SeatsAdult:     in.Filter.SeatsAdult,
			SeatsChild:     in.Filter.SeatsChild,
			SeatsInfant:    in.Filter.SeatsInfant,
			FXCode:         in.Filter.FXCode,
			Lang:           in.Filter.Lang,
			AgentID:        in.Filter.AgentID,
		})
	}
}

func intsToString(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}
