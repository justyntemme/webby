package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/justyntemme/webby/internal/auth"
	"github.com/justyntemme/webby/internal/models"
)

// StartReadingSession starts a new reading session
func (h *Handler) StartReadingSession(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req struct {
		BookID string `json:"book_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check if there's already an active session for this book
	existingSession, err := h.db.GetActiveReadingSession(userID, req.BookID)
	if err == nil && existingSession != nil {
		// Return existing session
		c.JSON(http.StatusOK, existingSession)
		return
	}

	// Create new session
	session := &models.ReadingSession{
		ID:        uuid.New().String(),
		UserID:    userID,
		BookID:    req.BookID,
		StartTime: time.Now(),
		CreatedAt: time.Now(),
	}

	if err := h.db.CreateReadingSession(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start session"})
		return
	}

	c.JSON(http.StatusCreated, session)
}

// EndReadingSession ends an active reading session
func (h *Handler) EndReadingSession(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	sessionID := c.Param("id")

	var req struct {
		PagesRead    int `json:"pages_read"`
		ChaptersRead int `json:"chapters_read"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body - just end the session
		req.PagesRead = 0
		req.ChaptersRead = 0
	}

	// Get the session
	session, err := h.db.GetActiveReadingSession(userID, sessionID)
	if err != nil {
		// Try to find by session ID in case bookID was passed
		c.JSON(http.StatusNotFound, gin.H{"error": "Active session not found"})
		return
	}

	// Calculate duration
	endTime := time.Now()
	duration := int(endTime.Sub(session.StartTime).Seconds())

	session.EndTime = &endTime
	session.PagesRead = req.PagesRead
	session.ChaptersRead = req.ChaptersRead
	session.DurationSeconds = duration

	if err := h.db.UpdateReadingSession(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to end session"})
		return
	}

	// Update daily stats
	h.db.UpdateDailyStats(userID, time.Now(), req.PagesRead, req.ChaptersRead, duration, session.BookID)

	// Update user statistics
	stats, _ := h.db.GetOrCreateUserStatistics(userID)
	if stats != nil {
		stats.TotalPagesRead += req.PagesRead
		stats.TotalChaptersRead += req.ChaptersRead
		stats.TotalTimeSeconds += duration
		now := time.Now()
		stats.LastReadingDate = &now

		// Update streak
		current, longest, _ := h.db.CalculateStreak(userID)
		stats.CurrentStreak = current
		if longest > stats.LongestStreak {
			stats.LongestStreak = longest
		}

		// Update completed books count
		completedCount, _ := h.db.GetCompletedBooksCount(userID)
		stats.TotalBooksRead = completedCount

		h.db.UpdateUserStatistics(stats)
	}

	c.JSON(http.StatusOK, session)
}

// UpdateReadingSessionProgress updates an active reading session's progress
func (h *Handler) UpdateReadingSessionProgress(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("bookId")

	var req struct {
		PagesRead    int `json:"pages_read"`
		ChaptersRead int `json:"chapters_read"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	session, err := h.db.GetActiveReadingSession(userID, bookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active session found"})
		return
	}

	session.PagesRead = req.PagesRead
	session.ChaptersRead = req.ChaptersRead

	if err := h.db.UpdateReadingSession(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// GetUserStatistics returns the user's reading statistics
func (h *Handler) GetUserStatistics(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	stats, err := h.db.GetOrCreateUserStatistics(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get statistics"})
		return
	}

	// Recalculate streak to ensure it's current
	current, longest, _ := h.db.CalculateStreak(userID)
	stats.CurrentStreak = current
	stats.LongestStreak = longest

	// Get completed books count
	completedCount, _ := h.db.GetCompletedBooksCount(userID)
	stats.TotalBooksRead = completedCount

	c.JSON(http.StatusOK, stats)
}

// GetDailyStats returns daily reading statistics for a date range
func (h *Handler) GetDailyStats(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Default to last 30 days
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	stats, err := h.db.GetDailyReadingStats(userID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get daily stats"})
		return
	}

	// Fill in missing days with zero values
	statsMap := make(map[string]models.DailyReadingStats)
	for _, s := range stats {
		statsMap[s.ReadingDate.Format("2006-01-02")] = s
	}

	var fullStats []map[string]interface{}
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		if s, ok := statsMap[dateStr]; ok {
			fullStats = append(fullStats, map[string]interface{}{
				"date":          dateStr,
				"pages_read":    s.PagesRead,
				"chapters_read": s.ChaptersRead,
				"time_seconds":  s.TimeSeconds,
				"books_touched": s.BooksTouched,
			})
		} else {
			fullStats = append(fullStats, map[string]interface{}{
				"date":          dateStr,
				"pages_read":    0,
				"chapters_read": 0,
				"time_seconds":  0,
				"books_touched": 0,
			})
		}
	}

	c.JSON(http.StatusOK, fullStats)
}

// GetRecentSessions returns recent reading sessions
func (h *Handler) GetRecentSessions(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	sessions, err := h.db.GetRecentReadingSessions(userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get sessions"})
		return
	}

	c.JSON(http.StatusOK, sessions)
}

// GetBookReadingStats returns reading statistics for a specific book
func (h *Handler) GetBookReadingStats(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")

	totalTime, pagesRead, sessionsCount, err := h.db.GetReadingStatsForBook(userID, bookID)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get book stats"})
		return
	}

	// Format time
	hours := totalTime / 3600
	minutes := (totalTime % 3600) / 60
	var timeFormatted string
	if hours > 0 {
		timeFormatted = strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
	} else if minutes > 0 {
		timeFormatted = strconv.Itoa(minutes) + "m"
	} else {
		timeFormatted = "< 1m"
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id":          bookID,
		"total_time":       totalTime,
		"time_formatted":   timeFormatted,
		"pages_read":       pagesRead,
		"sessions_count":   sessionsCount,
	})
}

// GetStatsSummary returns a quick summary of reading stats for the library page
func (h *Handler) GetStatsSummary(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	stats, err := h.db.GetOrCreateUserStatistics(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get statistics"})
		return
	}

	// Recalculate streak
	current, longest, _ := h.db.CalculateStreak(userID)

	// Get completed books count
	completedCount, _ := h.db.GetCompletedBooksCount(userID)

	// Format time
	hours := stats.TotalTimeSeconds / 3600
	minutes := (stats.TotalTimeSeconds % 3600) / 60
	var timeFormatted string
	if hours > 0 {
		timeFormatted = strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
	} else if minutes > 0 {
		timeFormatted = strconv.Itoa(minutes) + "m"
	} else {
		timeFormatted = "0m"
	}

	c.JSON(http.StatusOK, gin.H{
		"books_completed":     completedCount,
		"pages_read":          stats.TotalPagesRead,
		"total_time":          stats.TotalTimeSeconds,
		"total_time_formatted": timeFormatted,
		"current_streak":      current,
		"longest_streak":      longest,
	})
}
