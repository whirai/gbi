import { useEffect, useState } from 'react'
import { Checkbox, Divider, message, Modal, Spin } from 'antd'
import { getACL, setACL } from '../../../api/pages'
import * as groupsApi from '../../../api/groups'
import * as usersApi from '../../../api/users'
import type { Group, User } from '../../../api/types'

export default function AclModal({
  open,
  pageId,
  onClose,
  onDone,
}: {
  open: boolean
  pageId: number | null
  onClose: () => void
  onDone: () => void
}) {
  const [loading, setLoading] = useState(false)
  const [groups, setGroups] = useState<Group[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [selGroups, setSelGroups] = useState<number[]>([])
  const [selUsers, setSelUsers] = useState<number[]>([])

  useEffect(() => {
    if (!open || pageId === null) return
    setLoading(true)
    // 全量用户:分页拉取,避免 >100 用户时既有授权因选项缺失被静默丢弃
    Promise.all([getACL(pageId), groupsApi.list(), usersApi.listAllUsers()])
      .then(([acl, gs, us]) => {
        setGroups(gs)
        setUsers(us)
        setSelGroups(acl.filter((g) => g.subject_type === 'group').map((g) => g.subject_id))
        setSelUsers(acl.filter((g) => g.subject_type === 'user').map((g) => g.subject_id))
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => setLoading(false))
  }, [open, pageId])

  const save = async () => {
    if (pageId === null) return
    const grants = [
      ...selGroups.map((id) => ({ subject_type: 'group', subject_id: id })),
      ...selUsers.map((id) => ({ subject_type: 'user', subject_id: id })),
    ]
    try {
      await setACL(pageId, grants)
      message.success('权限已保存')
      onDone()
      onClose()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  return (
    <Modal title="页面查看权限" open={open} onOk={save} onCancel={onClose} width={560} destroyOnClose>
      <Spin spinning={loading}>
        <Divider orientation="left" plain>
          授权用户组
        </Divider>
        {groups.length === 0 ? (
          <div style={{ color: '#94a3b8' }}>暂无用户组</div>
        ) : (
          <Checkbox.Group
            options={groups.map((g) => ({ label: g.name, value: g.id }))}
            value={selGroups}
            onChange={(v) => setSelGroups(v as number[])}
          />
        )}
        <Divider orientation="left" plain>
          授权用户
        </Divider>
        <Checkbox.Group
          options={users.map((u) => ({ label: `${u.name} (${u.username})`, value: u.id }))}
          value={selUsers}
          onChange={(v) => setSelUsers(v as number[])}
        />
      </Spin>
    </Modal>
  )
}
