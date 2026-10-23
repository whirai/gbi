package service_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/config"
	"gbi/server/internal/service"
)

// newSubjectEnv 元数据用 newTestDB 的临时库;主题库文件落在独立的另一个临时目录。
func newSubjectEnv(t *testing.T) *service.SubjectService {
	d := newTestDB(t)
	dir := newTestDBDataDir(t)
	cfg := &config.Config{DBPath: dir + "/meta.db"}
	return service.NewSubjectService(d, cfg)
}

func TestSubjectCRUD(t *testing.T) {
	svc := newSubjectEnv(t)

	sub, err := svc.Create(service.SubjectCreate{Name: "销售主题", Code: "sales", Desc: "销售域"})
	require.NoError(t, err)
	require.NotEmpty(t, sub.DbFile)
	_, err = os.Stat(sub.DbFile)
	require.NoError(t, err) // 库文件已物化

	_, err = svc.Create(service.SubjectCreate{Name: "重复", Code: "sales"})
	require.ErrorIs(t, err, service.ErrDuplicateSubjectCode)

	_, err = svc.Create(service.SubjectCreate{Name: "非法", Code: "1bad"})
	require.ErrorIs(t, err, service.ErrInvalidSubjectCode)
	_, err = svc.Create(service.SubjectCreate{Name: "非法", Code: "Bad"})
	require.ErrorIs(t, err, service.ErrInvalidSubjectCode)

	list, err := svc.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	updated, err := svc.Update(sub.ID, service.SubjectUpdate{Name: "销售主题V2", Desc: "d2"})
	require.NoError(t, err)
	require.Equal(t, "销售主题V2", updated.Name)
	require.Equal(t, "sales", updated.Code) // code 不可改,接口也不接受

	got, err := svc.Get(sub.ID)
	require.NoError(t, err)
	require.Equal(t, sub.DbFile, got.DbFile)

	require.NoError(t, svc.Delete(sub.ID))
	_, err = os.Stat(sub.DbFile)
	require.True(t, os.IsNotExist(err)) // 库文件已删
	_, err = svc.Get(sub.ID)
	require.ErrorIs(t, err, service.ErrSubjectNotFound)
}
