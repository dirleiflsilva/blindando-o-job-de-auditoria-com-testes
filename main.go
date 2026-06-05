package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var (
	eventTypes = []string{"click", "view", "purchase", "login"}
	regions    = []string{"us-east-1", "eu-west-1", "ap-southeast-2", "sa-east-1"}
)

type Event struct {
	EventType string `json:"event_type"`
	Region    string `json:"region"`
}

type Report struct {
	TotalEvents int
	TotalErrors int
	ByEventType [4]int
	ByRegion    [4]int

	mu sync.Mutex
}

type ProcessResult struct {
	Event Event
	Err   error
}

type benchmarkResult struct {
	name     string
	report   *Report
	duration time.Duration
}

func NewReport() *Report {
	return &Report{}
}

func (r *Report) AddEvent(event Event) {
	r.TotalEvents++

	if idx := indexOf(eventTypes, event.EventType); idx >= 0 {
		r.ByEventType[idx]++
	}

	if idx := indexOf(regions, event.Region); idx >= 0 {
		r.ByRegion[idx]++
	}
}

func (r *Report) AddError() {
	r.TotalErrors++
}

func (r *Report) AddEventSafe(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.AddEvent(event)
}

func (r *Report) AddErrorSafe() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.AddError()
}

func ProcessSequential(files []string) *Report {
	report := NewReport()

	for _, filename := range files {
		processFile(filename, func(result ProcessResult) {
			if result.Err != nil {
				report.AddError()
				return
			}
			report.AddEvent(result.Event)
		})
	}

	return report
}

func ProcessConcurrentNaive(files []string) *Report {
	report := NewReport()
	var wg sync.WaitGroup

	for _, filename := range files {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			processFile(filename, func(result ProcessResult) {
				if result.Err != nil {
					// Intencionalmente inseguro: varias goroutines escrevem no mesmo Report.
					report.AddError()
					return
				}
				// Intencionalmente inseguro: use `go run -race .` para ver a DATA RACE.
				report.AddEvent(result.Event)
			})
		}(filename)
	}

	wg.Wait()
	return report
}

func ProcessConcurrentMutex(files []string) *Report {
	report := NewReport()
	var wg sync.WaitGroup

	for _, filename := range files {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			processFile(filename, func(result ProcessResult) {
				if result.Err != nil {
					report.AddErrorSafe()
					return
				}
				report.AddEventSafe(result.Event)
			})
		}(filename)
	}

	wg.Wait()
	return report
}

func ProcessPipeline(files []string, numWorkers int) *Report {
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan string, len(files))
	results := make(chan ProcessResult, 1000)
	report := NewReport()

	var workersWG sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for filename := range jobs {
				processFile(filename, func(result ProcessResult) {
					results <- result
				})
			}
		}()
	}

	var aggregatorWG sync.WaitGroup
	aggregatorWG.Add(1)
	go func() {
		defer aggregatorWG.Done()
		for result := range results {
			if result.Err != nil {
				report.AddError()
				continue
			}
			report.AddEvent(result.Event)
		}
	}()

	for _, filename := range files {
		jobs <- filename
	}
	close(jobs)

	workersWG.Wait()
	close(results)
	aggregatorWG.Wait()

	return report
}

func main() {
	logDir := getEnvString("AUDIT_LOG_DIR", "logs")
	numFiles := getEnvInt("AUDIT_NUM_FILES", 100)
	eventsPerFile := getEnvInt("AUDIT_EVENTS_PER_FILE", 10000)
	workers := getEnvInt("AUDIT_WORKERS", runtime.NumCPU())
	skipNaive := getEnvBool("AUDIT_SKIP_NAIVE")

	if err := GenerateMockFiles(logDir, numFiles, eventsPerFile); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao gerar arquivos mock: %v\n", err)
		os.Exit(1)
	}

	files, err := listGeneratedFiles(logDir, numFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao listar arquivos: %v\n", err)
		os.Exit(1)
	}

	sequential := measure("1. Sequencial", func() *Report { return ProcessSequential(files) })
	results := []benchmarkResult{sequential}

	var naive *benchmarkResult
	if !skipNaive {
		result := measure("2. Concorrente ingenuo (com race)", func() *Report { return ProcessConcurrentNaive(files) })
		naive = &result
		results = append(results, result)
	}

	mutex := measure("3. Concorrente com mutex", func() *Report { return ProcessConcurrentMutex(files) })
	pipeline := measure(fmt.Sprintf("4. Pipeline com worker pool (%d workers)", workers), func() *Report {
		return ProcessPipeline(files, workers)
	})
	results = append(results, mutex, pipeline)

	fmt.Printf("Arquivos: %d | Linhas por arquivo: %d | Workers: %d\n\n", len(files), eventsPerFile, workers)
	for _, result := range results {
		printResult(result)
	}

	fmt.Println()
	fmt.Printf("Parte 1 == Parte 3: %t\n", sameReport(sequential.report, mutex.report))
	fmt.Printf("Parte 1 == Parte 4: %t\n", sameReport(sequential.report, pipeline.report))
	if naive != nil {
		fmt.Printf("Parte 1 == Parte 2: %t (esperado: pode ser false por race condition)\n", sameReport(sequential.report, naive.report))
	} else {
		fmt.Println("Parte 2 ignorada por AUDIT_SKIP_NAIVE=true")
	}
}

func processFile(filename string, emit func(ProcessResult)) {
	file, err := os.Open(filename)
	if err != nil {
		emit(ProcessResult{Err: err})
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			emit(ProcessResult{Err: err})
			continue
		}
		emit(ProcessResult{Event: event})
	}

	if err := scanner.Err(); err != nil {
		emit(ProcessResult{Err: err})
	}
}

func measure(name string, process func() *Report) benchmarkResult {
	start := time.Now()
	report := process()
	return benchmarkResult{
		name:     name,
		report:   report,
		duration: time.Since(start),
	}
}

func printResult(result benchmarkResult) {
	report := result.report
	fmt.Printf("%-42s %10s | eventos=%d | erros=%d | tipos=%v | regioes=%v\n",
		result.name+":",
		result.duration.Round(time.Millisecond),
		report.TotalEvents,
		report.TotalErrors,
		report.ByEventType,
		report.ByRegion,
	)
}

func listGeneratedFiles(dir string, numFiles int) ([]string, error) {
	files := make([]string, 0, numFiles)
	for i := 0; i < numFiles; i++ {
		file := filepath.Join(dir, fmt.Sprintf("log_%03d.json", i))
		if _, err := os.Stat(file); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func sameReport(a, b *Report) bool {
	return a.TotalEvents == b.TotalEvents &&
		a.TotalErrors == b.TotalErrors &&
		reflect.DeepEqual(a.ByEventType, b.ByEventType) &&
		reflect.DeepEqual(a.ByRegion, b.ByRegion)
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func getEnvString(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}

	return value
}

func getEnvBool(name string) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && value
}
