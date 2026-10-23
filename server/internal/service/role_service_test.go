package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/service"
)

func TestRoleListAndRename(t *testing.T) {
	d := newTestDB(t)
	svc := service.NewRoleService(d)

	roles, err := svc.List()
	require.NoError(t, err)
	require.Len(t, roles, 3)
	require.Equal(t, []string{"admin", "developer", "user"},
		[]string{roles[0].Code, roles[1].Code, roles[2].Code})

	r, err := svc.Rename(roles[1].ID, "研发工程师")
	require.NoError(t, err)
	require.Equal(t, "研发工程师", r.Name)
	require.Equal(t, "developer", r.Code) // code 不变
}
