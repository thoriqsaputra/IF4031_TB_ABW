package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

const (
	defaultDBDSN         = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	defaultOpenSearchURL = "http://localhost:9200"
	defaultOpenSearchIdx = "reports"
	defaultInterval      = time.Minute
	defaultPageSize      = 1000
	defaultBatchSize     = 200
	httpTimeout          = 10 * time.Second
	defaultSyncTimeout   = 2 * time.Minute
)

type config struct {
	dbDSN           string
	openSearchURL   string
	openSearchIndex string
	interval        time.Duration
	pageSize        int
	batchSize       int
	syncTimeout     time.Duration
	username        string
	password        string
}

type openSearchClient struct {
	baseURL    string
	index      string
	httpClient *http.Client
	username   string
	password   string
}

type searchResponse struct {
	Hits struct {
		Hits []struct {
			ID string `json:"_id"`
		} `json:"hits"`
	} `json:"hits"`
}

func loadConfig() config {
	cfg := config{
		dbDSN:           defaultDBDSN,
		openSearchURL:   defaultOpenSearchURL,
		openSearchIndex: defaultOpenSearchIdx,
		interval:        defaultInterval,
		pageSize:        defaultPageSize,
		batchSize:       defaultBatchSize,
		syncTimeout:     defaultSyncTimeout,
	}

	if dsn := strings.TrimSpace(os.Getenv("DB_DSN")); dsn != "" {
		cfg.dbDSN = dsn
	}
	if rawURL := strings.TrimSpace(os.Getenv("OPENSEARCH_URL")); rawURL != "" {
		cfg.openSearchURL = rawURL
	}
	if index := strings.TrimSpace(os.Getenv("OPENSEARCH_INDEX")); index != "" {
		cfg.openSearchIndex = index
	}
	cfg.interval = parseDurationEnv("SYNC_INTERVAL", cfg.interval)
	cfg.pageSize = parseIntEnv("OPENSEARCH_PAGE_SIZE", cfg.pageSize)
	cfg.batchSize = parseIntEnv("DB_BATCH_SIZE", cfg.batchSize)
	cfg.syncTimeout = parseDurationEnv("SYNC_TIMEOUT", cfg.syncTimeout)
	cfg.username = strings.TrimSpace(os.Getenv("OPENSEARCH_USERNAME"))
	cfg.password = strings.TrimSpace(os.Getenv("OPENSEARCH_PASSWORD"))

	cfg.openSearchURL = strings.TrimRight(cfg.openSearchURL, "/")
	return cfg
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if value, err := time.ParseDuration(raw); err == nil {
		if value > 0 {
			return value
		}
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	log.Printf("Invalid %s=%q, using %s", key, raw, fallback)
	return fallback
}

func parseIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("Invalid %s=%q, using %d", key, raw, fallback)
		return fallback
	}
	return value
}

func connectDB(dsn string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	err = db.AutoMigrate(
		&models.Report{},
		&models.ReportCategory{},
		&models.ReportMedia{},
		&models.ReportAssignment{},
		&models.Upvote{},
		&models.Escalation{},
	)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connected")
	return db
}

func newOpenSearchClient(cfg config) *openSearchClient {
	return &openSearchClient{
		baseURL: cfg.openSearchURL,
		index:   cfg.openSearchIndex,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		username: cfg.username,
		password: cfg.password,
	}
}

func (c *openSearchClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
}

func (c *openSearchClient) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opensearch %s %s failed: %s", req.Method, req.URL.Path, strings.TrimSpace(string(payload)))
	}
	return resp, nil
}

func (c *openSearchClient) indexExists(ctx context.Context) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodHead, c.index, nil)
	if err != nil {
		return false, err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("opensearch HEAD %s failed: %s", req.URL.Path, strings.TrimSpace(string(payload)))
	}
	return true, nil
}

func (c *openSearchClient) ensureIndex(ctx context.Context) error {
	fmt.Println("Index checking...")
	exists, err := c.indexExists(ctx)
	fmt.Println("Index existence checked...")
	if err != nil {
		if createErr := c.createIndex(ctx); createErr == nil {
			return nil
		} else {
			return fmt.Errorf("opensearch ensure index failed: %v (create attempt failed: %v)", err, createErr)
		}
	}
	if exists {
		fmt.Println("Deemed existent...")
		return nil
	}
	fmt.Println("Attempting index creation...")
	return c.createIndex(ctx)
}

func (c *openSearchClient) createIndex(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodPut, c.index, bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		body := strings.TrimSpace(string(payload))
		if strings.Contains(body, "resource_already_exists_exception") {
			return nil
		}
		return fmt.Errorf("opensearch PUT %s failed: %s", req.URL.Path, body)
	}
	return nil
}

func (c *openSearchClient) countDocs(ctx context.Context) (uint64, error) {
	payload := map[string]interface{}{
		"query": map[string]interface{}{"match_all": map[string]interface{}{}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, c.index+"/_count", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Count uint64 `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}

	return parsed.Count, nil
}

func (c *openSearchClient) fetchExistingIDs(ctx context.Context, pageSize int) (map[uint]struct{}, error) {
	docCount, err := c.countDocs(ctx)
	if err != nil {
		return nil, err
	}
	if docCount == 0 {
		return map[uint]struct{}{}, nil
	}

	existing := make(map[uint]struct{})
	var lastID uint
	hasLastID := false
	for {
		payload := map[string]interface{}{
			"size":             pageSize,
			"sort":             []map[string]string{{"report_id": "asc"}},
			"_source":          false,
			"track_total_hits": false,
			"query":            map[string]interface{}{"match_all": map[string]interface{}{}},
		}
		if hasLastID {
			payload["search_after"] = []uint{lastID}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		req, err := c.newRequest(ctx, http.MethodPost, c.index+"/_search", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.do(req)
		if err != nil {
			return nil, err
		}

		var parsed searchResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		hits := parsed.Hits.Hits
		if len(hits) == 0 {
			break
		}
		for _, hit := range hits {
			if hit.ID == "" {
				continue
			}
			parsedID, err := strconv.ParseUint(hit.ID, 10, 64)
			if err != nil || parsedID == 0 {
				continue
			}
			existing[uint(parsedID)] = struct{}{}
		}

		parsedID, err := strconv.ParseUint(hits[len(hits)-1].ID, 10, 64)
		if err != nil || parsedID == 0 {
			break
		}
		lastID = uint(parsedID)
		hasLastID = true
	}

	return existing, nil
}

func (c *openSearchClient) bulkIndexReports(ctx context.Context, reports []models.Report) error {
	if len(reports) == 0 {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, report := range reports {
		if report.ReportID == 0 {
			continue
		}
		meta := map[string]map[string]string{
			"index": {
				"_index": c.index,
				"_id":    strconv.FormatUint(uint64(report.ReportID), 10),
			},
		}
		if err := enc.Encode(meta); err != nil {
			return err
		}
		if err := enc.Encode(report); err != nil {
			return err
		}
	}

	if buf.Len() == 0 {
		return nil
	}

	req, err := c.newRequest(ctx, http.MethodPost, "_bulk", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int             `json:"status"`
			Error  json.RawMessage `json:"error"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Errors {
		for _, item := range result.Items {
			for _, entry := range item {
				if entry.Status >= http.StatusBadRequest {
					return fmt.Errorf("bulk index failed with status %d: %s", entry.Status, strings.TrimSpace(string(entry.Error)))
				}
			}
		}
		return errors.New("bulk index reported errors")
	}

	return nil
}

func syncOnce(ctx context.Context, db *gorm.DB, client *openSearchClient, pageSize, batchSize int) error {
	existing, err := client.fetchExistingIDs(ctx, pageSize)
	if err != nil {
		return err
	}
	log.Printf("Found %d reports in OpenSearch", len(existing))

	var totalIndexed int
	var reports []models.Report
	err = db.WithContext(ctx).Order("report_id asc").FindInBatches(&reports, batchSize, func(tx *gorm.DB, batch int) error {
		missing := make([]models.Report, 0, len(reports))
		for _, report := range reports {
			if report.ReportID == 0 {
				continue
			}
			if _, ok := existing[report.ReportID]; ok {
				continue
			}
			missing = append(missing, report)
		}
		if len(missing) == 0 {
			return nil
		}
		if err := client.bulkIndexReports(ctx, missing); err != nil {
			return err
		}
		for _, doc := range missing {
			existing[doc.ReportID] = struct{}{}
		}
		totalIndexed += len(missing)
		log.Printf("Indexed batch %d (%d reports)", batch+1, len(missing))
		return nil
	}).Error
	if err != nil {
		return err
	}

	log.Printf("Sync complete. Indexed %d new reports", totalIndexed)
	return nil
}

func main() {
	cfg := loadConfig()
	db := connectDB(cfg.dbDSN)
	client := newOpenSearchClient(cfg)
	if err := client.ensureIndex(context.Background()); err != nil {
		log.Fatal("Failed to ensure OpenSearch index:", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("Report sync service started (interval=%s)", cfg.interval)

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	for {
		syncCtx, cancel := context.WithTimeout(ctx, cfg.syncTimeout)
		if err := syncOnce(syncCtx, db, client, cfg.pageSize, cfg.batchSize); err != nil {
			log.Printf("Sync error: %v", err)
		}
		cancel()

		select {
		case <-ctx.Done():
			log.Println("Shutting down")
			return
		case <-ticker.C:
		}
	}
}
