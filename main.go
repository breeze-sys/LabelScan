package main

import (
	"Label-Only-MIA-Go/pkg/attack"
	"Label-Only-MIA-Go/pkg/audit"
	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/dataset"
	"Label-Only-MIA-Go/pkg/mathutils"
	"Label-Only-MIA-Go/pkg/worker"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type runConfig struct {
	AuditMode                 string `json:"audit_mode"`
	Preset                    string `json:"preset"`
	TargetAPI                 string `json:"target_api"`
	ShadowAPI                 string `json:"shadow_api"`
	ShadowConfigPath          string `json:"shadow_config_path"`
	MemberIndexPath           string `json:"member_index_path"`
	MemberDataRoot            string `json:"member_data_root"`
	CalibrationDataPath       string `json:"calibration_data_path"`
	NonMemberDataPath         string `json:"non_member_data_path"`
	JSONReportPath            string `json:"json_report_path"`
	HTMLReportPath            string `json:"html_report_path"`
	CalibrationCandidateCount int    `json:"calibration_candidate_count"`
	CalibrationTargetCount    int    `json:"calibration_target_count"`
	MinValidStrangers         int    `json:"min_valid_strangers"`
	MemberSampleCount         int    `json:"member_sample_count"`
	NonMemberSampleCount      int    `json:"non_member_sample_count"`
	AuditWorkers              int    `json:"audit_workers"`
	MaxQueries                int    `json:"max_queries"`
	MaxIterations             int    `json:"max_iterations"`
	NumEvals                  int    `json:"num_evals"`
}

type metricSummary struct {
	Total          int     `json:"total"`
	MemberSamples  int     `json:"member_samples"`
	NonMembers     int     `json:"non_member_samples"`
	PredictedRisk  int     `json:"predicted_risk"`
	TruePositive   int     `json:"true_positive"`
	FalsePositive  int     `json:"false_positive"`
	TrueNegative   int     `json:"true_negative"`
	FalseNegative  int     `json:"false_negative"`
	Accuracy       float64 `json:"accuracy"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	HighRiskRate   float64 `json:"high_risk_rate"`
	MeanShadowLoss float64 `json:"mean_shadow_loss"`
	MeanDistance   float64 `json:"mean_boundary_distance"`
	MeanVolatility float64 `json:"mean_volatility_cv"`
}

type reportSample struct {
	SampleID      int     `json:"sample_id"`
	Label         int     `json:"label"`
	IsMemberTrue  bool    `json:"is_member_true"`
	PredictedRisk bool    `json:"predicted_risk"`
	RiskLevel     string  `json:"risk_level"`
	RiskClass     string  `json:"risk_class"`
	ShadowLoss    float64 `json:"shadow_loss"`
	MeanDistance  float64 `json:"mean_boundary_distance"`
	VolatilityCV  float64 `json:"volatility_cv"`
	Conclusion    string  `json:"conclusion"`
}

type auditReport struct {
	Tool        string                `json:"tool"`
	Version     string                `json:"version"`
	GeneratedAt string                `json:"generated_at"`
	Config      runConfig             `json:"config"`
	Thresholds  audit.AuditThresholds `json:"thresholds"`
	Metrics     metricSummary         `json:"metrics"`
	Samples     []reportSample        `json:"samples"`
}

type cliOptions struct {
	Serve bool
	Addr  string
}

type auditRequest struct {
	AuditMode        string `json:"audit_mode"`
	Preset           string `json:"preset"`
	TargetAPI        string `json:"target_api"`
	ShadowAPI        string `json:"shadow_api"`
	MemberSamples    int    `json:"member_samples"`
	NonMemberSamples int    `json:"non_member_samples"`
	Workers          int    `json:"workers"`
	MaxQueries       int    `json:"max_queries"`
	MaxIterations    int    `json:"max_iterations"`
	NumEvals         int    `json:"num_evals"`
	ConnectorMode    string `json:"connector_mode"`
	ExternalAPIHint  string `json:"external_api_hint"`
}

type serviceHealth struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Healthy   bool   `json:"healthy"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	CheckedAt string `json:"checked_at"`
}

type statusResponse struct {
	Target serviceHealth `json:"target"`
	Shadow serviceHealth `json:"shadow"`
}

type consolePageData struct {
	TargetAPI string
	ShadowAPI string
}

func defaultConfig() runConfig {
	return runConfig{
		AuditMode:                 envOrDefault("AUDIT_MODE", "full"),
		Preset:                    "standard",
		TargetAPI:                 envOrDefault("TARGET_API", "http://localhost:8000"),
		ShadowAPI:                 envOrDefault("SHADOW_API", "http://localhost:8001"),
		ShadowConfigPath:          envOrDefault("SHADOW_CONFIG_PATH", "shadow_config.json"),
		MemberIndexPath:           envOrDefault("MEMBER_INDEX_PATH", "target_members.json"),
		MemberDataRoot:            envOrDefault("MEMBER_DATA_ROOT", "data/cifar-10-batches-bin"),
		CalibrationDataPath:       envOrDefault("CALIBRATION_DATA_PATH", "data/cifar-10-batches-bin/test_batch.bin"),
		NonMemberDataPath:         envOrDefault("NON_MEMBER_DATA_PATH", "data/cifar-10-batches-bin/test_batch.bin"),
		JSONReportPath:            envOrDefault("JSON_REPORT_PATH", "output/audit_report.json"),
		HTMLReportPath:            envOrDefault("HTML_REPORT_PATH", "output/audit_report.html"),
		CalibrationCandidateCount: 100,
		CalibrationTargetCount:    10,
		MinValidStrangers:         5,
		MemberSampleCount:         50,
		NonMemberSampleCount:      50,
		AuditWorkers:              20,
		MaxQueries:                5000,
		MaxIterations:             40,
		NumEvals:                  100,
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func applyStandardPreset(cfg *runConfig) {
	cfg.CalibrationCandidateCount = 100
	cfg.CalibrationTargetCount = 10
	cfg.MinValidStrangers = 5
	cfg.MemberSampleCount = 50
	cfg.NonMemberSampleCount = 50
	cfg.AuditWorkers = 20
	cfg.MaxQueries = 5000
	cfg.MaxIterations = 40
	cfg.NumEvals = 100
}

func applyPreset(cfg *runConfig, preset string) error {
	normalized := strings.ToLower(strings.TrimSpace(preset))
	if normalized == "" {
		normalized = "standard"
	}
	cfg.Preset = normalized
	applyStandardPreset(cfg)
	switch normalized {
	case "smoke":
		cfg.CalibrationCandidateCount = 10
		cfg.CalibrationTargetCount = 1
		cfg.MinValidStrangers = 1
		cfg.MemberSampleCount = 1
		cfg.NonMemberSampleCount = 1
		cfg.AuditWorkers = 2
		cfg.MaxQueries = 800
		cfg.MaxIterations = 8
		cfg.NumEvals = 30
	case "standard":
	case "extended", "full":
		cfg.Preset = "extended"
		cfg.CalibrationCandidateCount = 200
		cfg.CalibrationTargetCount = 20
		cfg.MinValidStrangers = 10
		cfg.MemberSampleCount = 100
		cfg.NonMemberSampleCount = 100
	case "custom":
	default:
		return fmt.Errorf("unknown preset %q, choose smoke / standard / extended / custom", preset)
	}
	return nil
}

func normalizeAuditMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "full":
		return "full", nil
	case "boundary", "boundary-only", "boundary_only":
		return "boundary-only", nil
	default:
		return "", fmt.Errorf("unknown audit mode %q, choose full / boundary-only", mode)
	}
}

func loadConfigFromFlags() (runConfig, cliOptions) {
	cfg := defaultConfig()

	serve := flag.Bool("serve", false, "start local Web audit console")
	addr := flag.String("addr", envOrDefault("LABELSCAN_ADDR", ":8080"), "Web console listen address")
	auditMode := flag.String("audit-mode", cfg.AuditMode, "audit mode: full / boundary-only")
	preset := flag.String("preset", cfg.Preset, "audit preset: smoke / standard / extended / custom")
	targetAPI := flag.String("target-api", cfg.TargetAPI, "target model API")
	shadowAPI := flag.String("shadow-api", cfg.ShadowAPI, "shadow model API")
	shadowConfig := flag.String("shadow-config", cfg.ShadowConfigPath, "shadow threshold config")
	memberIndex := flag.String("member-index", cfg.MemberIndexPath, "target member index JSON")
	memberRoot := flag.String("member-root", cfg.MemberDataRoot, "CIFAR member batch root")
	calibrationPath := flag.String("calibration-data", cfg.CalibrationDataPath, "calibration data path")
	nonMemberPath := flag.String("non-member-data", cfg.NonMemberDataPath, "non-member data path")
	jsonReport := flag.String("json-report", cfg.JSONReportPath, "JSON report path")
	htmlReport := flag.String("html-report", cfg.HTMLReportPath, "HTML report path")
	calCandidates := flag.Int("calibration-candidates", cfg.CalibrationCandidateCount, "calibration candidate count")
	calTargets := flag.Int("calibration-targets", cfg.CalibrationTargetCount, "target valid calibration samples")
	minStrangers := flag.Int("min-valid-strangers", cfg.MinValidStrangers, "minimum valid calibration samples")
	memberCount := flag.Int("member-samples", cfg.MemberSampleCount, "member sample count")
	nonMemberCount := flag.Int("non-member-samples", cfg.NonMemberSampleCount, "non-member sample count")
	workers := flag.Int("workers", cfg.AuditWorkers, "audit workers")
	maxQueries := flag.Int("max-queries", cfg.MaxQueries, "HSJA max queries")
	maxIterations := flag.Int("max-iterations", cfg.MaxIterations, "HSJA max iterations")
	numEvals := flag.Int("num-evals", cfg.NumEvals, "HSJA evals per iteration")

	flag.Parse()
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if err := applyPreset(&cfg, *preset); err != nil {
		log.Fatal(err)
	}

	mode, err := normalizeAuditMode(*auditMode)
	if err != nil {
		log.Fatal(err)
	}
	cfg.AuditMode = mode

	cfg.TargetAPI = *targetAPI
	cfg.ShadowAPI = *shadowAPI
	cfg.ShadowConfigPath = *shadowConfig
	cfg.MemberIndexPath = *memberIndex
	cfg.MemberDataRoot = *memberRoot
	cfg.CalibrationDataPath = *calibrationPath
	cfg.NonMemberDataPath = *nonMemberPath
	cfg.JSONReportPath = *jsonReport
	cfg.HTMLReportPath = *htmlReport

	if explicit["calibration-candidates"] {
		cfg.CalibrationCandidateCount = *calCandidates
	}
	if explicit["calibration-targets"] {
		cfg.CalibrationTargetCount = *calTargets
	}
	if explicit["min-valid-strangers"] {
		cfg.MinValidStrangers = *minStrangers
	}
	if explicit["member-samples"] {
		cfg.MemberSampleCount = *memberCount
	}
	if explicit["non-member-samples"] {
		cfg.NonMemberSampleCount = *nonMemberCount
	}
	if explicit["workers"] {
		cfg.AuditWorkers = *workers
	}
	if explicit["max-queries"] {
		cfg.MaxQueries = *maxQueries
	}
	if explicit["max-iterations"] {
		cfg.MaxIterations = *maxIterations
	}
	if explicit["num-evals"] {
		cfg.NumEvals = *numEvals
	}

	if cfg.AuditWorkers < 1 {
		cfg.AuditWorkers = 1
	}
	if cfg.MemberSampleCount < 0 || cfg.NonMemberSampleCount < 0 {
		log.Fatal("sample counts cannot be negative")
	}

	return cfg, cliOptions{Serve: *serve, Addr: *addr}
}

func loadThresholds(path string) (audit.AuditThresholds, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return audit.AuditThresholds{}, err
	}
	var thresholds audit.AuditThresholds
	if err := json.Unmarshal(data, &thresholds); err != nil {
		return audit.AuditThresholds{}, err
	}
	return thresholds, nil
}

func calibrateThresholds(cfg runConfig, thresholds *audit.AuditThresholds, hsja *attack.HSJA, targetModel *client.HTTPClient) error {
	fmt.Printf("\n[1/4] Calibrating geometry signal with %d reference samples\n", cfg.CalibrationTargetCount)
	loader := &dataset.CifarLoader{}
	candidates, err := loader.GetRandomStrangers(cfg.CalibrationDataPath, cfg.CalibrationCandidateCount)
	if err != nil {
		return err
	}

	var refDists [][]float64
	validStrangers := 0
	for i := 0; i < len(candidates) && validStrangers < cfg.CalibrationTargetCount; i++ {
		s := candidates[i]
		resOrig := hsja.Attack(core.Sample{Data: s.Data, Label: s.Label}, targetModel)
		if resOrig.Distance < 1e-5 {
			continue
		}

		variants := mathutils.GenerateVariants(s.Data, 0.001, 10)
		points := append([][]float32{s.Data}, variants...)
		groupDists := make([]float64, 0, len(points))
		for _, img := range points {
			res := hsja.Attack(core.Sample{Data: img, Label: s.Label}, targetModel)
			groupDists = append(groupDists, res.Distance)
		}
		refDists = append(refDists, groupDists)
		validStrangers++
		fmt.Printf("  reference sample %d/%d ready\n", validStrangers, cfg.CalibrationTargetCount)
	}

	if validStrangers < cfg.MinValidStrangers {
		return fmt.Errorf("not enough valid calibration samples: %d < %d", validStrangers, cfg.MinValidStrangers)
	}

	thresholds.TauD, thresholds.TauCV = mathutils.CalibrateReference(refDists)
	return nil
}

func loadAuditSamples(cfg runConfig) ([]core.Sample, error) {
	fmt.Printf("\n[2/4] Loading audit samples: %d member + %d non-member\n", cfg.MemberSampleCount, cfg.NonMemberSampleCount)

	indexData, err := os.ReadFile(cfg.MemberIndexPath)
	if err != nil {
		return nil, fmt.Errorf("read member index %s: %w", cfg.MemberIndexPath, err)
	}
	var memberIndices []int
	if err := json.Unmarshal(indexData, &memberIndices); err != nil {
		return nil, err
	}
	if cfg.MemberSampleCount < len(memberIndices) {
		memberIndices = memberIndices[:cfg.MemberSampleCount]
	}

	memberLoader := &dataset.CifarLoader{IsMemberSet: true}
	members, err := memberLoader.LoadByIndices(cfg.MemberDataRoot, memberIndices)
	if err != nil {
		return nil, err
	}

	nonMemberLoader := &dataset.CifarLoader{IsMemberSet: false}
	nonMembers, err := nonMemberLoader.LoadBatch(cfg.NonMemberDataPath, cfg.NonMemberSampleCount)
	if err != nil {
		return nil, err
	}

	samples := append(members, nonMembers...)
	for i := range samples {
		samples[i].ID = i
	}
	return samples, nil
}

func isPredictedRisk(conclusion string) bool {
	return strings.Contains(conclusion, "🔴")
}

func riskLevel(conclusion string) (string, string) {
	switch {
	case strings.Contains(conclusion, "🔴"):
		return "高成员风险", "risk-red"
	case strings.Contains(conclusion, "🟡"):
		return "较高成员风险", "risk-yellow"
	case strings.Contains(conclusion, "🟠"):
		return "中等成员风险", "risk-orange"
	default:
		return "低成员风险", "risk-green"
	}
}

func buildReport(cfg runConfig, thresholds audit.AuditThresholds, results []core.AuditResult) auditReport {
	sort.Slice(results, func(i, j int) bool { return results[i].SampleID < results[j].SampleID })

	samples := make([]reportSample, 0, len(results))
	var metrics metricSummary
	metrics.Total = len(results)

	for _, r := range results {
		predRisk := isPredictedRisk(r.Conclusion)
		level, className := riskLevel(r.Conclusion)
		if r.IsMemberTrue {
			metrics.MemberSamples++
		} else {
			metrics.NonMembers++
		}
		if predRisk {
			metrics.PredictedRisk++
		}
		switch {
		case predRisk && r.IsMemberTrue:
			metrics.TruePositive++
		case predRisk && !r.IsMemberTrue:
			metrics.FalsePositive++
		case !predRisk && !r.IsMemberTrue:
			metrics.TrueNegative++
		case !predRisk && r.IsMemberTrue:
			metrics.FalseNegative++
		}
		metrics.MeanShadowLoss += r.ShadowLoss
		metrics.MeanDistance += r.MeanDistance
		metrics.MeanVolatility += r.VolatilityCV

		samples = append(samples, reportSample{
			SampleID:      r.SampleID,
			Label:         r.Label,
			IsMemberTrue:  r.IsMemberTrue,
			PredictedRisk: predRisk,
			RiskLevel:     level,
			RiskClass:     className,
			ShadowLoss:    r.ShadowLoss,
			MeanDistance:  r.MeanDistance,
			VolatilityCV:  r.VolatilityCV,
			Conclusion:    r.Conclusion,
		})
	}

	if metrics.Total > 0 {
		total := float64(metrics.Total)
		metrics.Accuracy = float64(metrics.TruePositive+metrics.TrueNegative) / total
		metrics.HighRiskRate = float64(metrics.PredictedRisk) / total
		metrics.MeanShadowLoss /= total
		metrics.MeanDistance /= total
		metrics.MeanVolatility /= total
	}
	if metrics.TruePositive+metrics.FalsePositive > 0 {
		metrics.Precision = float64(metrics.TruePositive) / float64(metrics.TruePositive+metrics.FalsePositive)
	}
	if metrics.TruePositive+metrics.FalseNegative > 0 {
		metrics.Recall = float64(metrics.TruePositive) / float64(metrics.TruePositive+metrics.FalseNegative)
	}

	return auditReport{
		Tool:        "LabelScan-Go",
		Version:     "web-console-v1",
		GeneratedAt: time.Now().Format(time.RFC3339),
		Config:      cfg,
		Thresholds:  thresholds,
		Metrics:     metrics,
		Samples:     samples,
	}
}

func runAudit(cfg runConfig) (auditReport, error) {
	mode, err := normalizeAuditMode(cfg.AuditMode)
	if err != nil {
		return auditReport{}, err
	}
	cfg.AuditMode = mode

	var thresholds audit.AuditThresholds
	if cfg.AuditMode == "full" {
		thresholds, err = loadThresholds(cfg.ShadowConfigPath)
		if err != nil {
			return auditReport{}, err
		}
	}

	targetModel := client.NewHTTPClient(cfg.TargetAPI)
	hsja := attack.NewHSJA(attack.HSJAConfig{
		MaxQueries:    cfg.MaxQueries,
		MaxIterations: cfg.MaxIterations,
		NumEvals:      cfg.NumEvals,
	})

	if err := calibrateThresholds(cfg, &thresholds, hsja, targetModel); err != nil {
		return auditReport{}, err
	}
	samples, err := loadAuditSamples(cfg)
	if err != nil {
		return auditReport{}, err
	}

	if cfg.AuditMode == "boundary-only" {
		fmt.Printf("\n[3/4] Running boundary-only audit with %d workers\n", cfg.AuditWorkers)
		results := runBoundaryOnlySamples(samples, cfg, thresholds, hsja, targetModel)
		return buildReport(cfg, thresholds, results), nil
	}

	fmt.Printf("\n[3/4] Running full label-only audit with %d workers\n", cfg.AuditWorkers)
	shadowModel := client.NewHTTPClient(cfg.ShadowAPI)
	engine := audit.NewEngine(thresholds, shadowModel, targetModel, hsja)
	pool := worker.NewAuditPool(engine, cfg.AuditWorkers)
	results := pool.RunAudit(samples)
	return buildReport(cfg, thresholds, results), nil
}

func runBoundaryOnlySamples(samples []core.Sample, cfg runConfig, thresholds audit.AuditThresholds, hsja *attack.HSJA, targetModel *client.HTTPClient) []core.AuditResult {
	workerCount := cfg.AuditWorkers
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(samples) && len(samples) > 0 {
		workerCount = len(samples)
	}

	jobs := make(chan core.Sample)
	results := make(chan core.AuditResult, len(samples))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sample := range jobs {
				results <- auditBoundaryOnlySample(sample, thresholds, hsja, targetModel)
			}
		}()
	}

	for _, sample := range samples {
		jobs <- sample
	}
	close(jobs)
	wg.Wait()
	close(results)

	collected := make([]core.AuditResult, 0, len(samples))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func auditBoundaryOnlySample(sample core.Sample, thresholds audit.AuditThresholds, hsja *attack.HSJA, targetModel *client.HTTPClient) core.AuditResult {
	res := core.AuditResult{
		SampleID:     sample.ID,
		Label:        sample.Label,
		IsMemberTrue: sample.IsMember,
	}

	origAtk := hsja.Attack(core.Sample{Data: sample.Data, Label: sample.Label}, targetModel)
	if origAtk.Distance < 1e-6 {
		res.MeanDistance = 0
		res.VolatilityCV = 99.0
		res.Conclusion = "🟢 低成员风险：目标模型未能正确识别原图，当前证据不足"
		return res
	}

	variants := mathutils.GenerateVariants(sample.Data, 0.001, 10)
	dists := make([]float64, len(variants)+1)
	dists[0] = origAtk.Distance

	var wg sync.WaitGroup
	for i, img := range variants {
		wg.Add(1)
		go func(idx int, variant core.Image) {
			defer wg.Done()
			atkRes := hsja.Attack(core.Sample{Data: variant, Label: sample.Label}, targetModel)
			dists[idx+1] = atkRes.Distance
		}(i, img)
	}
	wg.Wait()

	dBar, std := mathutils.MeanAndStd(dists)
	cv := 99.0
	if dBar > 0 {
		cv = std / dBar
	}
	res.MeanDistance = dBar
	res.VolatilityCV = cv

	isFar := dBar > thresholds.TauD
	isStable := cv < thresholds.TauCV
	switch {
	case isFar && isStable:
		res.Conclusion = "🔴 高成员风险：边界距离异常且局部扰动下保持稳定（Boundary-only 弱证据模式）"
	case isFar:
		res.Conclusion = "🟡 较高成员风险：边界距离异常（Boundary-only 弱证据模式）"
	case isStable:
		res.Conclusion = "🟡 较高成员风险：局部边界表现稳定（Boundary-only 弱证据模式）"
	default:
		res.Conclusion = "🟢 低成员风险：当前边界证据不足以支持成员推断（Boundary-only 弱证据模式）"
	}
	return res
}

func writeJSONReport(path string, report auditReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeHTMLReport(path string, report auditReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"pct": func(v float64) string { return fmt.Sprintf("%.1f%%", v*100) },
		"num": func(v float64) string { return fmt.Sprintf("%.4f", v) },
	}).Parse(htmlReportTemplate)
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return tmpl.Execute(file, report)
}

func printSummary(report auditReport) {
	fmt.Println("\n[4/4] Audit complete")
	fmt.Println("-----------------------------------------------------")
	fmt.Printf("Samples: %d (%d member / %d non-member)\n", report.Metrics.Total, report.Metrics.MemberSamples, report.Metrics.NonMembers)
	fmt.Printf("High-risk samples: %d\n", report.Metrics.PredictedRisk)
	fmt.Printf("Accuracy: %.2f%%\n", report.Metrics.Accuracy*100)
	fmt.Printf("Precision: %.2f%%\n", report.Metrics.Precision*100)
	fmt.Printf("Recall: %.2f%%\n", report.Metrics.Recall*100)
	fmt.Printf("Mean shadow loss: %.4f\n", report.Metrics.MeanShadowLoss)
	fmt.Printf("Mean boundary distance: %.4f\n", report.Metrics.MeanDistance)
	fmt.Println("-----------------------------------------------------")
}

func checkHealth(name, baseURL string) serviceHealth {
	result := serviceHealth{
		Name:      name,
		URL:       baseURL,
		Status:    "unreachable",
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/health")
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 300
	if result.Healthy {
		result.Status = "ok"
	} else {
		result.Status = fmt.Sprintf("http_%d", resp.StatusCode)
	}
	return result
}

func startWebServer(cfg runConfig, addr string) error {
	consoleTmpl, err := template.New("console").Parse(webConsoleTemplate)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = consoleTmpl.Execute(w, consolePageData{
			TargetAPI: cfg.TargetAPI,
			ShadowAPI: cfg.ShadowAPI,
		})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		targetAPI := r.URL.Query().Get("target_api")
		shadowAPI := r.URL.Query().Get("shadow_api")
		auditMode := r.URL.Query().Get("audit_mode")
		if targetAPI == "" {
			targetAPI = cfg.TargetAPI
		}
		if shadowAPI == "" {
			shadowAPI = cfg.ShadowAPI
		}
		mode, err := normalizeAuditMode(auditMode)
		if err != nil {
			mode = cfg.AuditMode
		}
		resp := statusResponse{
			Target: checkHealth("target", targetAPI),
		}
		if mode == "boundary-only" {
			resp.Shadow = serviceHealth{
				Name:      "shadow",
				URL:       shadowAPI,
				Healthy:   true,
				Status:    "skipped",
				Detail:    "boundary-only mode does not require shadow logits",
				CheckedAt: time.Now().Format(time.RFC3339),
			}
		} else {
			resp.Shadow = checkHealth("shadow", shadowAPI)
		}
		writeJSON(w, resp, http.StatusOK)
	})

	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req auditRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusBadRequest)
			return
		}
		runCfg := cfg
		preset := req.Preset
		if preset == "" {
			preset = "smoke"
		}
		if err := applyPreset(&runCfg, preset); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusBadRequest)
			return
		}
		if req.AuditMode != "" {
			runCfg.AuditMode = req.AuditMode
		}
		mode, err := normalizeAuditMode(runCfg.AuditMode)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusBadRequest)
			return
		}
		runCfg.AuditMode = mode
		if req.TargetAPI != "" {
			runCfg.TargetAPI = req.TargetAPI
		}
		if req.ShadowAPI != "" {
			runCfg.ShadowAPI = req.ShadowAPI
		}
		if runCfg.Preset == "custom" {
			if req.MemberSamples > 0 {
				runCfg.MemberSampleCount = req.MemberSamples
			}
			if req.NonMemberSamples > 0 {
				runCfg.NonMemberSampleCount = req.NonMemberSamples
			}
			if req.Workers > 0 {
				runCfg.AuditWorkers = req.Workers
			}
			if req.MaxQueries > 0 {
				runCfg.MaxQueries = req.MaxQueries
			}
			if req.MaxIterations > 0 {
				runCfg.MaxIterations = req.MaxIterations
			}
			if req.NumEvals > 0 {
				runCfg.NumEvals = req.NumEvals
			}
		}
		runCfg.JSONReportPath = "output/web_audit_report.json"
		runCfg.HTMLReportPath = "output/web_audit_report.html"

		report, err := runAudit(runCfg)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		if err := writeJSONReport(runCfg.JSONReportPath, report); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		if err := writeHTMLReport(runCfg.HTMLReportPath, report); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
			return
		}
		writeJSON(w, report, http.StatusOK)
	})

	mux.HandleFunc("/reports/latest.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "output/web_audit_report.html")
	})

	fmt.Printf("LabelScan-Go Web console listening on %s\n", publicListenURL(addr))
	return http.ListenAndServe(addr, mux)
}

func publicListenURL(addr string) string {
	switch {
	case strings.HasPrefix(addr, ":"):
		return "http://localhost" + addr
	case strings.HasPrefix(addr, "0.0.0.0:"):
		return "http://localhost:" + strings.TrimPrefix(addr, "0.0.0.0:")
	default:
		return "http://" + addr
	}
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	cfg, opts := loadConfigFromFlags()

	if opts.Serve {
		if err := startWebServer(cfg, opts.Addr); err != nil {
			log.Fatal(err)
		}
		return
	}

	fmt.Println("=====================================================")
	fmt.Println("LabelScan-Go: Label-Only MIA Audit Tool")
	fmt.Println("=====================================================")
	fmt.Printf("Preset: %s\nTarget: %s\nShadow: %s\n", cfg.Preset, cfg.TargetAPI, cfg.ShadowAPI)

	report, err := runAudit(cfg)
	if err != nil {
		log.Fatalf("audit failed: %v", err)
	}
	if err := writeJSONReport(cfg.JSONReportPath, report); err != nil {
		log.Fatalf("write JSON report failed: %v", err)
	}
	if err := writeHTMLReport(cfg.HTMLReportPath, report); err != nil {
		log.Fatalf("write HTML report failed: %v", err)
	}

	printSummary(report)
	fmt.Printf("JSON report: %s\n", cfg.JSONReportPath)
	if cfg.HTMLReportPath != "" {
		fmt.Printf("HTML report: %s\n", cfg.HTMLReportPath)
	}
}

const webConsoleTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>LabelScan-Go Risk Console</title>
  <style>
    :root { --bg:#eef6ff; --ink:#102033; --muted:#65758b; --panel:#ffffff; --line:#d8e6f7; --nav:#0b4ea2; --accent:#1677ff; --accent-2:#2f9bff; --soft:#eaf4ff; --green:#0b8f6a; --red:#d14343; --amber:#b7791f; --orange:#e16b2d; --shadow:0 24px 70px rgba(28,92,170,.16); }
    * { box-sizing:border-box; }
    body { margin:0; font-family:"Nunito","Quicksand","HarmonyOS Sans SC","MiSans","PingFang SC","Microsoft YaHei",sans-serif; color:var(--ink); background:
      radial-gradient(circle at 18% -4%, rgba(22,119,255,.20), transparent 30%),
      radial-gradient(circle at 84% 8%, rgba(47,155,255,.18), transparent 34%),
      linear-gradient(180deg,#f8fbff 0%,#eef6ff 48%,#e8f2ff 100%); }
    header { background:linear-gradient(135deg,#0753bd 0%,#1677ff 58%,#72c5ff 100%); color:#fff; padding:38px 42px 44px; position:relative; overflow:hidden; }
    header:before { content:""; position:absolute; left:-120px; bottom:-170px; width:420px; height:420px; border-radius:50%; background:rgba(255,255,255,.12); }
    header:after { content:""; position:absolute; right:-80px; top:-120px; width:390px; height:390px; border-radius:50%; border:1px solid rgba(255,255,255,.22); background:rgba(255,255,255,.10); }
    .hero { position:relative; max-width:1180px; margin:0 auto; }
    .eyebrow { display:inline-flex; gap:8px; align-items:center; border:1px solid rgba(255,255,255,.42); color:#e9f6ff; border-radius:999px; padding:7px 14px; font-size:12px; font-weight:900; letter-spacing:.09em; text-transform:uppercase; background:rgba(255,255,255,.12); }
    header h1 { margin:18px 0 12px; font-size:50px; line-height:1.04; letter-spacing:-.05em; font-weight:950; text-shadow:0 14px 34px rgba(0,46,116,.26); }
    header p { margin:0; color:#edf7ff; max-width:650px; line-height:1.58; font-size:17px; font-weight:650; }
    main { max-width:1240px; margin:0 auto; padding:24px; display:grid; grid-template-columns:390px 1fr; gap:18px; }
    section { background:rgba(255,255,255,.94); border:1px solid var(--line); border-radius:24px; overflow:hidden; box-shadow:var(--shadow); }
    h2 { margin:0; padding:18px 20px; border-bottom:1px solid var(--line); font-size:19px; letter-spacing:-.01em; background:linear-gradient(180deg,#f7fbff,#eef7ff); }
    .body { padding:18px 20px 20px; }
    label { display:block; font-size:12px; color:#5f7190; margin:13px 0 7px; font-weight:850; letter-spacing:.02em; }
    input, select { width:100%; height:44px; border:1px solid #c7dcf5; border-radius:16px; padding:0 13px; font:inherit; font-weight:650; background:#fbfdff; color:var(--ink); outline:none; }
    input:focus, select:focus { border-color:var(--accent); box-shadow:0 0 0 4px rgba(22,119,255,.13); }
    button { min-height:44px; border:0; border-radius:16px; padding:0 16px; font-weight:900; cursor:pointer; transition:transform .15s ease, box-shadow .15s ease; font-family:inherit; }
    button:hover { transform:translateY(-1px); }
    button:disabled { cursor:not-allowed; opacity:.6; transform:none; }
    .primary { background:linear-gradient(135deg,#0b66df,#22a3ff); color:#fff; width:100%; margin-top:17px; box-shadow:0 14px 30px rgba(22,119,255,.28); }
    .secondary { background:#e8f3ff; color:#123a70; border:1px solid #c7dcf5; }
    .row { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
    .status { display:grid; gap:9px; margin-top:13px; }
    .badge { display:inline-flex; align-items:center; gap:6px; padding:5px 9px; border-radius:999px; font-size:12px; font-weight:800; white-space:nowrap; }
    .ok { color:#fff; background:linear-gradient(135deg,#0b8f6a,#22b98e); }
    .bad { color:#fff; background:var(--red); }
    .hint { color:var(--muted); font-size:13px; line-height:1.65; margin-top:10px; }
    .notice { border:1px solid #b9dafd; background:#eff8ff; color:#155092; border-radius:18px; padding:13px 15px; line-height:1.6; font-size:13px; font-weight:650; }
    .cards { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; }
    .card { border:1px solid #d4e6fa; border-radius:20px; padding:17px; background:linear-gradient(180deg,#ffffff,#edf7ff); }
    .metric { font-size:30px; font-weight:950; margin-top:6px; letter-spacing:-.03em; color:#0b5ecf; }
    .table-wrap { margin-top:16px; overflow:auto; border:1px solid var(--line); border-radius:20px; background:#fff; }
    table { width:100%; border-collapse:collapse; font-size:13px; }
    th, td { padding:12px 13px; border-bottom:1px solid #e4dccd; text-align:left; white-space:nowrap; }
    th { background:#eef7ff; color:#405b7b; font-weight:900; }
    tr:last-child td { border-bottom:0; }
    .risk-red { color:#fff; background:var(--red); }
    .risk-yellow { color:#3b2a00; background:#f4cc4f; }
    .risk-orange { color:#fff; background:var(--orange); }
    .risk-green { color:#fff; background:var(--green); }
    .split { display:flex; gap:10px; align-items:center; margin-top:12px; flex-wrap:wrap; }
    a { color:#0b66df; font-weight:900; text-decoration:none; }
    pre { white-space:pre-wrap; background:#102033; color:#eaf5ff; padding:14px; border-radius:18px; max-height:220px; overflow:auto; font-size:12px; line-height:1.55; }
    code { background:#e8f3ff; padding:2px 6px; border-radius:8px; color:#0b5ecf; }
    @media (max-width:960px) { main { grid-template-columns:1fr; } .cards { grid-template-columns:repeat(2,minmax(0,1fr)); } }
    @media (max-width:620px) { header { padding:32px 22px; } header h1 { font-size:38px; } main { padding:14px; } .cards, .row { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <header>
    <div class="hero">
      <div class="eyebrow">Label-Only Membership Risk Audit</div>
      <h1>LabelScan-Go 风险评估控制台</h1>
      <p>本地运行的成员风险审计工具，输出可复核的风险等级与证据指标。</p>
    </div>
  </header>
  <main>
    <section>
      <h2>任务配置</h2>
      <div class="body">
        <div class="notice">输出结果为成员风险等级。由于目标模型准确率、数据分布和查询预算都会影响结果，建议结合证据指标进行人工复核。</div>
        <label>样本规模</label>
        <select id="preset">
          <option value="smoke">smoke：快速自检</option>
          <option value="standard">standard：常规评估</option>
          <option value="extended">extended：扩展评估</option>
          <option value="custom">custom：自定义</option>
        </select>
        <p class="hint" id="presetHint"></p>
        <label>审计模式</label>
        <select id="auditMode">
          <option value="full">Full：边界信号 + 影子模型信号</option>
          <option value="boundary-only">Boundary-only：仅使用目标模型边界信号</option>
        </select>
        <label>审计对象</label>
        <select id="targetSource">
          <option value="builtin-docker">内置 Docker 模型服务</option>
          <option value="external-compatible-api">外部兼容 API</option>
        </select>
        <label>Target API</label>
        <input id="targetApi" value="{{.TargetAPI}}">
        <label>Shadow API</label>
        <input id="shadowApi" value="{{.ShadowAPI}}">
        <div class="row">
          <div><label>成员样本</label><input id="memberSamples" type="number" min="1" value="1"></div>
          <div><label>非成员样本</label><input id="nonMemberSamples" type="number" min="1" value="1"></div>
        </div>
        <div class="row">
          <div><label>并发数</label><input id="workers" type="number" min="1" value="2"></div>
          <div><label>最大查询</label><input id="maxQueries" type="number" min="100" value="800"></div>
        </div>
        <button class="primary" id="runBtn">运行风险评估</button>
        <div class="split">
          <button class="secondary" id="statusBtn">检查服务状态</button>
          <a id="reportLink" href="/reports/latest.html" target="_blank" style="display:none;">打开 HTML 报告</a>
        </div>
        <div class="status" id="status"></div>
      </div>
    </section>
    <section>
      <h2>外部 API 接入</h2>
      <div class="body">
        <p class="hint">外部 API 可以作为正式审计对象接入。Boundary-only 模式只要求 Target 服务提供 label-only 分类接口，适合作为初筛；Full 模式还需要 Shadow 服务提供 logits 以及匹配的阈值配置，通常需要用户基于同任务数据自行训练或校准。</p>
        <label>外部 API 说明</label>
        <input id="externalHint" value="compatible label-only classifier API">
        <p class="hint">最低兼容要求：Target API 提供 <code>/predict</code>、<code>/predict_batch</code> 和 <code>/health</code>；Shadow API 在 Full 模式下额外提供 <code>/predict_logits</code>。</p>
      </div>
    </section>
    <section style="grid-column:1 / -1;">
      <h2>风险评估结果</h2>
      <div class="body">
        <div class="cards">
          <div class="card"><div class="hint">准确率</div><div class="metric" id="acc">--</div></div>
          <div class="card"><div class="hint">高风险查准率</div><div class="metric" id="precision">--</div></div>
          <div class="card"><div class="hint">成员召回率</div><div class="metric" id="recall">--</div></div>
          <div class="card"><div class="hint">高风险占比</div><div class="metric" id="riskRate">--</div></div>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>ID</th><th>参考成员标记</th><th>风险等级</th><th>类别标签</th><th>Shadow Loss</th><th>边界距离</th><th>波动系数</th><th>证据摘要</th></tr></thead>
            <tbody id="sampleRows"><tr><td colspan="8">尚未运行风险评估</td></tr></tbody>
          </table>
        </div>
        <pre id="log">系统就绪。请先检查服务状态，再运行风险评估。</pre>
      </div>
    </section>
  </main>
  <script>
    const $ = id => document.getElementById(id);
    const pct = v => (v * 100).toFixed(1) + '%';
    const num = v => Number(v || 0).toFixed(4);
    const presetDefaults = {
      smoke: {memberSamples: 1, nonMemberSamples: 1, workers: 2, maxQueries: 800, hint: '快速自检：1 个成员样本、1 个非成员样本，最大 800 次查询。'},
      standard: {memberSamples: 50, nonMemberSamples: 50, workers: 20, maxQueries: 5000, hint: '常规评估：50 个成员样本、50 个非成员样本，最大 5000 次查询。'},
      extended: {memberSamples: 100, nonMemberSamples: 100, workers: 20, maxQueries: 5000, hint: '扩展评估：100 个成员样本、100 个非成员样本，并使用更充分的校准样本。'}
    };
    const manualFieldIds = ['memberSamples', 'nonMemberSamples', 'workers', 'maxQueries'];
    function syncPresetControls() {
      const preset = $('preset').value;
      const defaults = presetDefaults[preset];
      if (defaults) {
        $('memberSamples').value = defaults.memberSamples;
        $('nonMemberSamples').value = defaults.nonMemberSamples;
        $('workers').value = defaults.workers;
        $('maxQueries').value = defaults.maxQueries;
      }
      const isCustom = preset === 'custom';
      manualFieldIds.forEach(id => { $(id).disabled = !isCustom; });
      $('presetHint').textContent = isCustom
        ? '自定义模式：页面中的样本数、并发数和最大查询数会作为运行参数提交。'
        : defaults.hint + ' 非 custom 模式下手动参数不会覆盖预设。';
    }
    function payload() {
      const data = {
        audit_mode: $('auditMode').value,
        preset: $('preset').value,
        target_api: $('targetApi').value,
        shadow_api: $('shadowApi').value,
        connector_mode: $('targetSource').value,
        external_api_hint: $('externalHint').value
      };
      if ($('preset').value === 'custom') {
        data.member_samples = Number($('memberSamples').value);
        data.non_member_samples = Number($('nonMemberSamples').value);
        data.workers = Number($('workers').value);
        data.max_queries = Number($('maxQueries').value);
      }
      return data;
    }
    async function checkStatus() {
      const q = new URLSearchParams({target_api: $('targetApi').value, shadow_api: $('shadowApi').value, audit_mode: $('auditMode').value});
      const res = await fetch('/api/status?' + q.toString());
      const data = await res.json();
      $('status').innerHTML = [data.target, data.shadow].map(s =>
        '<div><span class="badge '+(s.healthy?'ok':'bad')+'">'+s.name+' '+s.status+'</span> <span class="hint">'+s.url+'</span></div>'
      ).join('');
    }
    async function runAudit() {
      $('runBtn').disabled = true;
      $('log').textContent = '风险评估进行中。边界搜索需要多次查询目标模型，请保持容器服务运行。';
      $('reportLink').style.display = 'none';
      try {
        const res = await fetch('/api/audit', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload())});
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'audit failed');
        $('acc').textContent = pct(data.metrics.accuracy);
        $('precision').textContent = pct(data.metrics.precision);
        $('recall').textContent = pct(data.metrics.recall);
        $('riskRate').textContent = pct(data.metrics.high_risk_rate);
        $('sampleRows').innerHTML = data.samples.map(s =>
          '<tr><td>'+s.sample_id+'</td><td>'+s.is_member_true+'</td><td><span class="badge '+s.risk_class+'">'+s.risk_level+'</span></td><td>'+s.label+'</td><td>'+num(s.shadow_loss)+'</td><td>'+num(s.mean_boundary_distance)+'</td><td>'+num(s.volatility_cv)+'</td><td>'+s.conclusion+'</td></tr>'
        ).join('');
        $('log').textContent = JSON.stringify(data.metrics, null, 2);
        $('reportLink').style.display = 'inline';
      } catch (err) {
        $('log').textContent = err.message;
      } finally {
        $('runBtn').disabled = false;
      }
    }
    $('statusBtn').addEventListener('click', checkStatus);
    $('runBtn').addEventListener('click', runAudit);
    $('auditMode').addEventListener('change', checkStatus);
    $('preset').addEventListener('change', syncPresetControls);
    syncPresetControls();
    checkStatus();
  </script>
</body>
</html>`

const htmlReportTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>LabelScan-Go Risk Assessment Report</title>
  <style>
    :root { color-scheme: light; --ink:#102033; --muted:#65758b; --line:#d8e6f7; --bg:#eef6ff; --panel:#ffffff; --red:#d14343; --yellow:#9a6700; --orange:#e16b2d; --green:#0b8f6a; }
    body { margin:0; font-family:"Nunito","Quicksand","HarmonyOS Sans SC","MiSans","PingFang SC","Microsoft YaHei",sans-serif; color:var(--ink); background:linear-gradient(180deg,#f8fbff,#eef6ff); }
    header { padding:44px 44px 34px; background:linear-gradient(135deg,#0753bd,#1677ff 58%,#72c5ff); color:#fff; }
    header h1 { margin:0 0 10px; font-size:44px; font-weight:950; letter-spacing:-.05em; text-shadow:0 12px 30px rgba(0,46,116,.25); }
    header p { margin:0; color:#edf7ff; line-height:1.65; font-weight:650; }
    main { max-width:1180px; margin:0 auto; padding:28px 24px 48px; }
    .grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:14px; margin-bottom:22px; }
    .card, .section { background:var(--panel); border:1px solid var(--line); border-radius:22px; box-shadow:0 18px 54px rgba(28,92,170,.12); }
    .card { padding:17px; }
    .label { color:var(--muted); font-size:13px; margin-bottom:8px; }
    .value { font-size:30px; font-weight:950; color:#0b5ecf; }
    .section { margin-top:18px; overflow:hidden; }
    .section h2 { font-size:20px; margin:0; padding:18px 20px; border-bottom:1px solid var(--line); background:#eef7ff; letter-spacing:-.02em; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th, td { padding:12px 13px; border-bottom:1px solid var(--line); text-align:left; vertical-align:top; }
    th { color:#405b7b; background:#eef7ff; font-weight:900; }
    tr:last-child td { border-bottom:0; }
    .pill { display:inline-block; padding:4px 9px; border-radius:999px; font-weight:800; font-size:12px; white-space:nowrap; }
    .risk-red { color:#fff; background:var(--red); }
    .risk-yellow { color:#402d00; background:#facc15; }
    .risk-orange { color:#fff; background:var(--orange); }
    .risk-green { color:#fff; background:var(--green); }
    .meta { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px 24px; padding:16px 18px; font-size:14px; }
    .meta span { color:var(--muted); }
    .notice { margin-top:18px; padding:15px 18px; border:1px solid #b9dafd; background:#eff8ff; color:#155092; border-radius:18px; line-height:1.65; font-weight:650; }
    @media (max-width:860px) { .grid { grid-template-columns:repeat(2,minmax(0,1fr)); } .meta { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <header>
    <h1>LabelScan-Go 风险评估报告</h1>
    <p>生成时间 {{.GeneratedAt}} · 样本规模 {{.Config.Preset}} · 审计模式 {{.Config.AuditMode}}</p>
  </header>
  <main>
    <section class="grid">
      <div class="card"><div class="label">准确率</div><div class="value">{{pct .Metrics.Accuracy}}</div></div>
      <div class="card"><div class="label">高风险查准率</div><div class="value">{{pct .Metrics.Precision}}</div></div>
      <div class="card"><div class="label">成员召回率</div><div class="value">{{pct .Metrics.Recall}}</div></div>
      <div class="card"><div class="label">高风险占比</div><div class="value">{{pct .Metrics.HighRiskRate}}</div></div>
    </section>
    <div class="notice">本报告输出成员风险等级和相关证据指标，不表示对训练集归属作确定性判定。Boundary-only 模式仅使用边界几何证据，建议作为初筛结果解读。</div>
    <section class="section">
      <h2>运行摘要</h2>
      <div class="meta">
        <div><span>Target API:</span> {{.Config.TargetAPI}}</div>
        <div><span>Shadow API:</span> {{.Config.ShadowAPI}}</div>
        <div><span>Audit mode:</span> {{.Config.AuditMode}}</div>
        <div><span>Samples:</span> {{.Metrics.Total}} total, {{.Metrics.MemberSamples}} member, {{.Metrics.NonMembers}} non-member</div>
        <div><span>Workers:</span> {{.Config.AuditWorkers}}</div>
        <div><span>Shadow thresholds:</span> red {{num .Thresholds.Tau95}}, yellow {{num .Thresholds.TauOpt}}</div>
        <div><span>Geometry thresholds:</span> distance {{num .Thresholds.TauD}}, volatility {{num .Thresholds.TauCV}}</div>
      </div>
    </section>
    <section class="section">
      <h2>样本风险明细</h2>
      <table>
        <thead><tr><th>ID</th><th>参考成员标记</th><th>风险等级</th><th>类别标签</th><th>Shadow Loss</th><th>边界距离</th><th>波动系数</th><th>证据摘要</th></tr></thead>
        <tbody>
          {{range .Samples}}
          <tr>
            <td>{{.SampleID}}</td>
            <td>{{.IsMemberTrue}}</td>
            <td><span class="pill {{.RiskClass}}">{{.RiskLevel}}</span></td>
            <td>{{.Label}}</td>
            <td>{{num .ShadowLoss}}</td>
            <td>{{num .MeanDistance}}</td>
            <td>{{num .VolatilityCV}}</td>
            <td>{{.Conclusion}}</td>
          </tr>
          {{end}}
        </tbody>
      </table>
    </section>
  </main>
</body>
</html>`
