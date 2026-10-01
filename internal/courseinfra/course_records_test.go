package courseinfra

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type fakeDNS struct {
	zoneID   string
	records  []RecordUpsertRequest
	verified string
}

func (f *fakeDNS) AddDomain(_ context.Context, _ string, _ string) (string, error) {
	return f.zoneID, nil
}

func (f *fakeDNS) UpsertRecord(_ context.Context, record RecordUpsertRequest, _ string) error {
	f.records = append(f.records, record)
	return nil
}

func (f *fakeDNS) VerifyDomain(_ context.Context, domain string, _ string) error {
	f.verified = domain
	return nil
}

func TestCourseDNSManagerApply(t *testing.T) {
	deadline := time.Date(2027, time.March, 14, 16, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name        string
		input       CourseDNSInput
		wantNames   []string
		wantContent []string
	}{
		{
			name: "publishes delivery deadline and educator report records",
			input: CourseDNSInput{
				CourseID: "physics-204", Domain: "school.example", CourseHost: "physics",
				DeliveryTarget: "courses.example.net", Deadline: deadline, ReportEmail: "faculty@school.example",
			},
			wantNames:   []string{"physics", "_course-deadline.physics", "_educator-report.physics"},
			wantContent: []string{"courses.example.net", "2027-03-14T08:00:00Z", "faculty@school.example"},
		},
		{
			name: "keeps another course isolated by host name",
			input: CourseDNSInput{
				CourseID: "history-101", Domain: "school.example", CourseHost: "history",
				DeliveryTarget: "archive.example.net", Deadline: deadline, ReportEmail: "history@school.example",
			},
			wantNames:   []string{"history", "_course-deadline.history", "_educator-report.history"},
			wantContent: []string{"archive.example.net", "2027-03-14T08:00:00Z", "history@school.example"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeDNS{zoneID: "zone-test-42"}
			result, err := NewCourseDNSManager(api).Apply(context.Background(), test.input)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if result.ZoneID != "zone-test-42" {
				t.Fatalf("ZoneID = %q", result.ZoneID)
			}
			if !reflect.DeepEqual(result.Names, test.wantNames) {
				t.Fatalf("names = %#v, want %#v", result.Names, test.wantNames)
			}
			gotContent := make([]string, len(api.records))
			for index, record := range api.records {
				if record.ZoneID != "zone-test-42" {
					t.Fatalf("record %d zone_id = %q", index, record.ZoneID)
				}
				gotContent[index] = record.Content
			}
			if !reflect.DeepEqual(gotContent, test.wantContent) {
				t.Fatalf("content = %#v, want %#v", gotContent, test.wantContent)
			}
			if api.verified != test.input.Domain {
				t.Fatalf("verified domain = %q", api.verified)
			}
		})
	}
}
