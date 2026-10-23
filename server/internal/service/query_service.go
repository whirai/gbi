package service

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/datacore"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

type QueryService struct{ db *gorm.DB }

func NewQueryService(db *gorm.DB) *QueryService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewQueryService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewQueryService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &QueryService{db: db}
}

type QueryRequest struct {
	SQL        string `json:"sql"`
	SubjectIDs []uint `json:"subject_ids"`
}

type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

var ErrNoSubjectSelected = errors.New("请至少选择一个数据主题")

func (s *QueryService) Run(in QueryRequest) (res *QueryResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "QueryService.Run", "subjects", len(in.SubjectIDs), "sql_len", len(in.SQL))
	defer func() {
		slog.Debug("方法离开", "fn", "QueryService.Run", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("QueryService.Run开始", "subject_ids", in.SubjectIDs, "sql_len", len(in.SQL), "sql_preview", logger.Truncate(in.SQL, 200))
	if len(in.SubjectIDs) == 0 {
		slog.Debug("QueryService.Run错误", "err", ErrNoSubjectSelected)
		return nil, ErrNoSubjectSelected
	}
	refs := make([]datacore.SubjectRef, 0, len(in.SubjectIDs))
	for _, id := range in.SubjectIDs {
		var sub model.Subject
		if err := s.db.First(&sub, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Debug("QueryService.Run错误", "err", ErrSubjectNotFound)
				return nil, ErrSubjectNotFound
			}
			slog.Debug("QueryService.Run错误", "err", err)
			return nil, err
		}
		refs = append(refs, datacore.SubjectRef{Code: sub.Code, File: sub.DbFile})
	}
	cols, rows, err := datacore.ExecuteQuery(refs, in.SQL)
	if err != nil {
		slog.Debug("QueryService.Run错误", "err", err)
		return nil, err
	}
	slog.Debug("QueryService.Run成功", "rows", len(rows))
	return &QueryResult{Columns: cols, Rows: rows}, nil
}
