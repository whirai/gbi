package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/service"
)

func TestDeptTreeCRUD(t *testing.T) {
	d := newTestDB(t)
	svc := service.NewDeptService(d)

	root, err := svc.Create("总公司", nil, 0)
	require.NoError(t, err)
	child, err := svc.Create("华东", &root.ID, 1)
	require.NoError(t, err)

	tree, err := svc.Tree()
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Equal(t, "总公司", tree[0].Name)
	require.Len(t, tree[0].Children, 1)
	require.Equal(t, "华东", tree[0].Children[0].Name)

	renamed, err := svc.Update(child.ID, "华东区", 2)
	require.NoError(t, err)
	require.Equal(t, "华东区", renamed.Name)
}

func TestDeptDeleteGuards(t *testing.T) {
	d := newTestDB(t)
	svc := service.NewDeptService(d)
	root, _ := svc.Create("总公司", nil, 0)
	child, _ := svc.Create("华东", &root.ID, 0)

	err := svc.Delete(root.ID)
	require.ErrorIs(t, err, service.ErrDeptHasChildren)

	require.NoError(t, svc.Delete(child.ID))
	require.NoError(t, svc.Delete(root.ID))
}
