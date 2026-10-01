package courseinfra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

type CourseDNSInput struct {
	CourseID       string
	Domain         string
	CourseHost     string
	DeliveryTarget string
	Deadline       time.Time
	ReportEmail    string
}

type ApplyResult struct {
	ZoneID string
	Names  []string
}

type DNSAPI interface {
	AddDomain(context.Context, string, string) (string, error)
	UpsertRecord(context.Context, RecordUpsertRequest, string) error
	VerifyDomain(context.Context, string, string) error
}

type CourseDNSManager struct {
	dns DNSAPI
}

func NewCourseDNSManager(dns DNSAPI) *CourseDNSManager {
	return &CourseDNSManager{dns: dns}
}

func (m *CourseDNSManager) Apply(ctx context.Context, input CourseDNSInput) (ApplyResult, error) {
	if err := validateInput(input); err != nil {
		return ApplyResult{}, err
	}
	requestID := stableID(input)
	zoneID, err := m.dns.AddDomain(ctx, input.Domain, requestID+"-domain")
	if err != nil {
		return ApplyResult{}, err
	}

	records := courseRecords(zoneID, input)
	result := ApplyResult{ZoneID: zoneID, Names: make([]string, 0, len(records))}
	for index, record := range records {
		if err := m.dns.UpsertRecord(ctx, record, fmt.Sprintf("%s-record-%d", requestID, index)); err != nil {
			return ApplyResult{}, err
		}
		result.Names = append(result.Names, record.Name)
	}
	if err := m.dns.VerifyDomain(ctx, input.Domain, requestID+"-verify"); err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

func courseRecords(zoneID string, input CourseDNSInput) []RecordUpsertRequest {
	metadata := map[string]any{"course_id": input.CourseID}
	return []RecordUpsertRequest{
		{
			ZoneID: zoneID, RecordType: "CNAME", Name: input.CourseHost,
			Content: input.DeliveryTarget, TTL: 300, Metadata: metadata,
		},
		{
			ZoneID: zoneID, RecordType: "TXT", Name: "_course-deadline." + input.CourseHost,
			Content: input.Deadline.UTC().Format(time.RFC3339), TTL: 300, Metadata: metadata,
		},
		{
			ZoneID: zoneID, RecordType: "TXT", Name: "_educator-report." + input.CourseHost,
			Content: input.ReportEmail, TTL: 300, Metadata: metadata,
		},
	}
}

func validateInput(input CourseDNSInput) error {
	if strings.TrimSpace(input.CourseID) == "" || strings.TrimSpace(input.Domain) == "" {
		return fmt.Errorf("course_id and domain are required")
	}
	if strings.TrimSpace(input.CourseHost) == "" || strings.TrimSpace(input.DeliveryTarget) == "" {
		return fmt.Errorf("course_host and delivery_target are required")
	}
	if input.Deadline.IsZero() {
		return fmt.Errorf("deadline is required")
	}
	if _, err := mail.ParseAddress(input.ReportEmail); err != nil {
		return fmt.Errorf("report_email must be an email address")
	}
	return nil
}

func stableID(input CourseDNSInput) string {
	source := strings.Join([]string{
		input.CourseID, input.Domain, input.CourseHost, input.DeliveryTarget,
		input.Deadline.UTC().Format(time.RFC3339), input.ReportEmail,
	}, "|")
	sum := sha256.Sum256([]byte(source))
	return "course-dns-" + hex.EncodeToString(sum[:8])
}
