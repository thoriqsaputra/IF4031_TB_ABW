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
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"middleware"
	"models"
)

const (
	defaultDBDSN         = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	defaultOpenSearchURL = "http://localhost:9200"
	defaultOpenSearchIdx = "reports"
	defaultPageSize      = 20
	defaultMaxPageSize   = 100
	httpTimeout          = 10 * time.Second
)

type config struct {
	dbDSN           string
	readDBDSN       string
	openSearchURL   string
	openSearchIndex string
	pageSize        int
	maxPageSize     int
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

type reportDocument struct {
	ReportID    uint      `json:"report_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public"`
	UserID      uint      `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type searchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source reportDocument `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (r searchResponse) total() int {
	if r.Hits.Total.Value > 0 {
		return r.Hits.Total.Value
	}
	return len(r.Hits.Hits)
}

type reportEnvelope struct {
	ReportID    uint
	Title       string
	Description string
	IsPublic    bool
	UserID      uint
	UserName    string
	CreatedAt   time.Time
}

type reportSummary struct {
	ReportID          uint   `json:"report_id"`
	PosterName        string `json:"poster_name"`
	ReportTitle       string `json:"report_title"`
	ReportDescription string `json:"report_description"`
	IsPublic          bool   `json:"is_public"`
}

type ReportFormatter interface {
	Format([]reportEnvelope) []reportSummary
}

// FormatterFunc adapts a function to ReportFormatter.
type FormatterFunc func([]reportEnvelope) []reportSummary

func (f FormatterFunc) Format(reports []reportEnvelope) []reportSummary {
	return f(reports)
}

var (
	DB              *gorm.DB
	searchClient    *openSearchClient
	searchConfig    config
	reportFormatter ReportFormatter = FormatterFunc(defaultSummaryFormatter)
)

func defaultSummaryFormatter(reports []reportEnvelope) []reportSummary {
	summaries := make([]reportSummary, 0, len(reports))
	for _, report := range reports {
		summaries = append(summaries, reportSummary{
			ReportID:          report.ReportID,
			PosterName:        report.UserName,
			ReportTitle:       report.Title,
			ReportDescription: report.Description,
			IsPublic:          report.IsPublic,
		})
	}
	return summaries
}

func loadConfig() config {
	cfg := config{
		dbDSN:           defaultDBDSN,
		openSearchURL:   defaultOpenSearchURL,
		openSearchIndex: defaultOpenSearchIdx,
		pageSize:        defaultPageSize,
		maxPageSize:     defaultMaxPageSize,
	}
	cfg.readDBDSN = cfg.dbDSN

	if dsn := strings.TrimSpace(os.Getenv("DB_DSN")); dsn != "" {
		cfg.dbDSN = dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("READ_DB_DSN")); dsn != "" {
		cfg.readDBDSN = dsn
	} else {
		cfg.readDBDSN = cfg.dbDSN
	}
	if rawURL := strings.TrimSpace(os.Getenv("OPENSEARCH_URL")); rawURL != "" {
		cfg.openSearchURL = rawURL
	}
	if index := strings.TrimSpace(os.Getenv("OPENSEARCH_INDEX")); index != "" {
		cfg.openSearchIndex = index
	}
	cfg.pageSize = parseIntEnv("SEARCH_PAGE_SIZE", cfg.pageSize)
	cfg.maxPageSize = parseIntEnv("SEARCH_MAX_PAGE_SIZE", cfg.maxPageSize)
	cfg.username = strings.TrimSpace(os.Getenv("OPENSEARCH_USERNAME"))
	cfg.password = strings.TrimSpace(os.Getenv("OPENSEARCH_PASSWORD"))

	if cfg.maxPageSize < cfg.pageSize {
		cfg.maxPageSize = cfg.pageSize
	}

	cfg.openSearchURL = strings.TrimRight(cfg.openSearchURL, "/")
	return cfg
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

func (c *openSearchClient) search(ctx context.Context, payload map[string]interface{}) (searchResponse, error) {
	var parsed searchResponse
	body, err := json.Marshal(payload)
	if err != nil {
		return parsed, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, c.index+"/_search", bytes.NewReader(body))
	if err != nil {
		return parsed, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return parsed, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func buildSearchPayload(query string, filters []map[string]interface{}, offset, limit int) map[string]interface{} {
	must := make([]map[string]interface{}, 0, 1)
	if strings.TrimSpace(query) == "" {
		must = append(must, map[string]interface{}{"match_all": map[string]interface{}{}})
	} else {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"title^2", "description", "location"},
			},
		})
	}

	return map[string]interface{}{
		"from":             offset,
		"size":             limit,
		"track_total_hits": true,
		"sort": []map[string]string{
			{"created_at": "desc"},
		},
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filters,
			},
		},
	}
}

func parsePagination(c *fiber.Ctx, cfg config) (int, int, error) {
	limit := cfg.pageSize
	offset := 0

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, 0, errors.New("invalid limit")
		}
		limit = value
	} else if raw := strings.TrimSpace(c.Query("size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, 0, errors.New("invalid size")
		}
		limit = value
	}

	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return 0, 0, errors.New("invalid offset")
		}
		offset = value
	} else if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return 0, 0, errors.New("invalid from")
		}
		offset = value
	}

	if limit > cfg.maxPageSize {
		limit = cfg.maxPageSize
	}
	return offset, limit, nil
}

func fetchUserNames(ctx context.Context, db *gorm.DB, userIDs []uint) (map[uint]string, error) {
	names := make(map[uint]string)
	if len(userIDs) == 0 {
		return names, nil
	}

	var users []models.User
	if err := db.WithContext(ctx).Select("user_id", "name").Where("user_id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		names[user.UserID] = user.Name
	}
	return names, nil
}

func handleSearch(c *fiber.Ctx, filters []map[string]interface{}) error {
	query := strings.TrimSpace(c.Query("q"))
	offset, limit, err := parsePagination(c, searchConfig)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	ctx := context.Background()
	payload := buildSearchPayload(query, filters, offset, limit)
	resp, err := searchClient.search(ctx, payload)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	docs := make([]reportDocument, 0, len(resp.Hits.Hits))
	userSet := make(map[uint]struct{})
	for _, hit := range resp.Hits.Hits {
		doc := hit.Source
		docs = append(docs, doc)
		if doc.UserID > 0 {
			userSet[doc.UserID] = struct{}{}
		}
	}

	userIDs := make([]uint, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}

	userNames, err := fetchUserNames(ctx, DB, userIDs)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	reports := make([]reportEnvelope, 0, len(docs))
	for _, doc := range docs {
		name := userNames[doc.UserID]
		if name == "" {
			name = "Unknown"
		}
		reports = append(reports, reportEnvelope{
			ReportID:    doc.ReportID,
			Title:       doc.Title,
			Description: doc.Description,
			IsPublic:    doc.IsPublic,
			UserID:      doc.UserID,
			UserName:    name,
			CreatedAt:   doc.CreatedAt,
		})
	}

	items := reportFormatter.Format(reports)
	total := resp.total()
	nextOffset := offset + len(items)
	var nextOffsetValue interface{} = nil
	if nextOffset < total {
		nextOffsetValue = nextOffset
	}
	return c.JSON(fiber.Map{
		"total":       total,
		"count":       len(items),
		"limit":       limit,
		"offset":      offset,
		"next_offset": nextOffsetValue,
		"items":       items,
	})
}

func userIDFromLocals(c *fiber.Ctx) (uint, error) {
	raw := c.Locals("userID")
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case float32:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case int:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case int64:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case uint:
		if v == 0 {
			return 0, errors.New("invalid user id")
		}
		return v, nil
	case uint64:
		if v == 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case string:
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil || parsed == 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(parsed), nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil || parsed <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(parsed), nil
	default:
		return 0, errors.New("invalid user id")
	}
}

func SearchPublic(c *fiber.Ctx) error {
	filters := []map[string]interface{}{
		{"term": map[string]interface{}{"is_public": true}},
	}
	return handleSearch(c, filters)
}

func SearchSelf(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}
	filters := []map[string]interface{}{
		{"term": map[string]interface{}{"user_id": userID}},
	}
	return handleSearch(c, filters)
}

func main() {
	searchConfig = loadConfig()
	DB = connectDB(searchConfig.readDBDSN)
	searchClient = newOpenSearchClient(searchConfig)

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "*",
	}))

	app.Get("/searchbar_public", SearchPublic)
	app.Get("/searchbar_self", middleware.Protected(), SearchSelf)

	log.Fatal(app.Listen(":3003"))
}
