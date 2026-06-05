package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReportAddition(t *testing.T) {
	report := NewReport()

	report.AddEvent(Event{EventType: "click", Region: "us-east-1"})
	report.AddEvent(Event{EventType: "view", Region: "eu-west-1"})
	report.AddEvent(Event{EventType: "click", Region: "eu-west-1"})
	report.AddEvent(Event{EventType: "unknown", Region: "unknown"})
	report.AddError()
	report.AddError()

	if report.TotalEvents != 4 {
		t.Fatalf("TotalEvents = %d, want 4", report.TotalEvents)
	}
	if report.TotalErrors != 2 {
		t.Fatalf("TotalErrors = %d, want 2", report.TotalErrors)
	}

	wantByEventType := [4]int{2, 1, 0, 0}
	if report.ByEventType != wantByEventType {
		t.Errorf("ByEventType = %v, want %v", report.ByEventType, wantByEventType)
	}

	wantByRegion := [4]int{1, 2, 0, 0}
	if report.ByRegion != wantByRegion {
		t.Errorf("ByRegion = %v, want %v", report.ByRegion, wantByRegion)
	}
}

func TestReportConcurrency(t *testing.T) {
	const (
		goroutines       = 1000
		eventsPerRoutine = 3
		errorsPerRoutine = 2
	)

	report := NewReport()
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			for j := 0; j < eventsPerRoutine; j++ {
				report.AddEventSafe(Event{
					EventType: eventTypes[(i+j)%len(eventTypes)],
					Region:    regions[(i+j)%len(regions)],
				})
			}

			for j := 0; j < errorsPerRoutine; j++ {
				report.AddErrorSafe()
			}
		}(i)
	}

	wg.Wait()

	wantTotalEvents := goroutines * eventsPerRoutine
	if report.TotalEvents != wantTotalEvents {
		t.Fatalf("TotalEvents = %d, want %d", report.TotalEvents, wantTotalEvents)
	}

	wantTotalErrors := goroutines * errorsPerRoutine
	if report.TotalErrors != wantTotalErrors {
		t.Fatalf("TotalErrors = %d, want %d", report.TotalErrors, wantTotalErrors)
	}

	wantByEventType := [4]int{750, 750, 750, 750}
	if report.ByEventType != wantByEventType {
		t.Errorf("ByEventType = %v, want %v", report.ByEventType, wantByEventType)
	}

	wantByRegion := [4]int{750, 750, 750, 750}
	if report.ByRegion != wantByRegion {
		t.Errorf("ByRegion = %v, want %v", report.ByRegion, wantByRegion)
	}
}

func TestProcessFileHelper(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantTotalEvents int
		wantTotalErrors int
		wantByEventType [4]int
		wantByRegion    [4]int
	}{
		{
			name: "only valid events",
			content: `{"event_type":"click","region":"us-east-1"}
{"event_type":"purchase","region":"sa-east-1"}
{"event_type":"login","region":"sa-east-1"}
`,
			wantTotalEvents: 3,
			wantTotalErrors: 0,
			wantByEventType: [4]int{1, 0, 1, 1},
			wantByRegion:    [4]int{1, 0, 0, 2},
		},
		{
			name: "valid and invalid lines",
			content: `{"event_type":"view","region":"eu-west-1"}
this is not valid json
{"event_type":"click","region":"ap-southeast-2"}
`,
			wantTotalEvents: 2,
			wantTotalErrors: 1,
			wantByEventType: [4]int{1, 1, 0, 0},
			wantByRegion:    [4]int{0, 1, 1, 0},
		},
		{
			name: "unknown dimensions count as event only",
			content: `{"event_type":"unknown","region":"unknown"}
{"event_type":"login","region":"us-east-1"}
`,
			wantTotalEvents: 2,
			wantTotalErrors: 0,
			wantByEventType: [4]int{0, 0, 0, 1},
			wantByRegion:    [4]int{1, 0, 0, 0},
		},
		{
			name:            "missing file emits error",
			wantTotalEvents: 0,
			wantTotalErrors: 1,
			wantByEventType: [4]int{},
			wantByRegion:    [4]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewReport()
			filename := filepath.Join(t.TempDir(), "audit.log")

			if tt.name == "missing file emits error" {
				filename = filepath.Join(t.TempDir(), "missing.log")
			} else if err := os.WriteFile(filename, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			processFile(filename, func(result ProcessResult) {
				if result.Err != nil {
					report.AddError()
					return
				}
				report.AddEvent(result.Event)
			})

			assertReport(t, report, tt.wantTotalEvents, tt.wantTotalErrors, tt.wantByEventType, tt.wantByRegion)
		})
	}
}

func TestProcessPipelineMatchesSequential(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateMockFiles(dir, 4, 120); err != nil {
		t.Fatalf("GenerateMockFiles failed: %v", err)
	}

	files, err := listGeneratedFiles(dir, 4)
	if err != nil {
		t.Fatalf("listGeneratedFiles failed: %v", err)
	}

	sequential := ProcessSequential(files)
	pipeline := ProcessPipeline(files, 3)

	if !sameReport(sequential, pipeline) {
		t.Fatalf("pipeline report = %+v, want sequential report %+v", pipeline, sequential)
	}
}

func assertReport(t *testing.T, report *Report, wantTotalEvents, wantTotalErrors int, wantByEventType, wantByRegion [4]int) {
	t.Helper()

	if report.TotalEvents != wantTotalEvents {
		t.Fatalf("TotalEvents = %d, want %d", report.TotalEvents, wantTotalEvents)
	}
	if report.TotalErrors != wantTotalErrors {
		t.Fatalf("TotalErrors = %d, want %d", report.TotalErrors, wantTotalErrors)
	}
	if report.ByEventType != wantByEventType {
		t.Errorf("ByEventType = %v, want %v", report.ByEventType, wantByEventType)
	}
	if report.ByRegion != wantByRegion {
		t.Errorf("ByRegion = %v, want %v", report.ByRegion, wantByRegion)
	}
}
