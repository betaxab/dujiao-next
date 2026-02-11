package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetDashboardOverview 获取后台仪表盘总览
func (h *Handler) GetDashboardOverview(c *gin.Context) {
	input, err := parseDashboardQuery(c)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.DashboardService.GetOverview(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.dashboard_fetch_failed", err)
		return
	}

	response.Success(c, data)
}

// GetDashboardRankings 获取后台仪表盘排行榜
func (h *Handler) GetDashboardRankings(c *gin.Context) {
	input, err := parseDashboardQuery(c)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.DashboardService.GetRankings(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.dashboard_fetch_failed", err)
		return
	}

	response.Success(c, data)
}

// GetDashboardTrends 获取后台仪表盘趋势
func (h *Handler) GetDashboardTrends(c *gin.Context) {
	input, err := parseDashboardQuery(c)
	if err != nil {
		respondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.DashboardService.GetTrends(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			respondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		respondError(c, response.CodeInternal, "error.dashboard_fetch_failed", err)
		return
	}

	response.Success(c, data)
}

func parseDashboardQuery(c *gin.Context) (service.DashboardQueryInput, error) {
	rangeRaw := strings.TrimSpace(c.DefaultQuery("range", "7d"))
	fromRaw := strings.TrimSpace(c.Query("from"))
	toRaw := strings.TrimSpace(c.Query("to"))
	timezone := strings.TrimSpace(c.Query("tz"))
	forceRefreshRaw := strings.TrimSpace(c.Query("force_refresh"))

	from, err := parseTimeNullable(fromRaw)
	if err != nil {
		return service.DashboardQueryInput{}, err
	}
	to, err := parseTimeNullable(toRaw)
	if err != nil {
		return service.DashboardQueryInput{}, err
	}

	forceRefresh := false
	if forceRefreshRaw != "" {
		parsed, err := strconv.ParseBool(forceRefreshRaw)
		if err != nil {
			return service.DashboardQueryInput{}, err
		}
		forceRefresh = parsed
	}

	return service.DashboardQueryInput{
		Range:        rangeRaw,
		From:         from,
		To:           to,
		Timezone:     timezone,
		ForceRefresh: forceRefresh,
	}, nil
}
