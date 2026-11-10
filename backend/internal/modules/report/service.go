package report

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidDateRange = errors.New("invalid date range")

// Service provides business logic and validation for platform sales and revenue reports.
type Service struct {
	repo *Repository
}

// NewService creates a new report service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetRevenueSummary validates optional date bounds and returns aggregated revenue metrics.
func (s *Service) GetRevenueSummary(ctx context.Context, startStr, endStr string) (*RevenueSummary, error) {
	filter, err := parseDateRange(startStr, endStr, 0)
	if err != nil {
		return nil, err
	}
	return s.repo.GetRevenueSummary(ctx, filter)
}

// GetDailyRevenueBreakdown validates parameters and returns daily revenue points.
func (s *Service) GetDailyRevenueBreakdown(ctx context.Context, startStr, endStr string, limit int) ([]DailyRevenuePoint, error) {
	filter, err := parseDateRange(startStr, endStr, limit)
	if err != nil {
		return nil, err
	}
	return s.repo.GetDailyRevenueBreakdown(ctx, filter)
}

// GetSellerRevenueReport validates parameters and returns per-seller net revenue aggregations.
func (s *Service) GetSellerRevenueReport(ctx context.Context, startStr, endStr string, limit int) ([]SellerSalesReport, error) {
	filter, err := parseDateRange(startStr, endStr, limit)
	if err != nil {
		return nil, err
	}
	return s.repo.GetSellerRevenueReport(ctx, filter)
}

func parseDateRange(startStr, endStr string, limit int) (DateRangeFilter, error) {
	var filter DateRangeFilter
	filter.Limit = limit

	startStr = strings.TrimSpace(startStr)
	endStr = strings.TrimSpace(endStr)

	if startStr != "" {
		parsedStart, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			return filter, fmt.Errorf("%w: start_date must be YYYY-MM-DD", ErrInvalidDateRange)
		}
		utcStart := parsedStart.UTC()
		filter.StartDate = &utcStart
	}

	if endStr != "" {
		parsedEnd, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			return filter, fmt.Errorf("%w: end_date must be YYYY-MM-DD", ErrInvalidDateRange)
		}
		// Include the entire end day up to 23:59:59.999999999 UTC
		utcEnd := parsedEnd.UTC().Add(24*time.Hour - time.Nanosecond)
		filter.EndDate = &utcEnd
	}

	if filter.StartDate != nil && filter.EndDate != nil && filter.StartDate.After(*filter.EndDate) {
		return filter, fmt.Errorf("%w: start_date cannot be after end_date", ErrInvalidDateRange)
	}

	return filter, nil
}
