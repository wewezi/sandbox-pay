import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Chip, Modal, TextField, InputGroup } from '@heroui/react'
import {
  Store, Plus, RefreshCw, Copy, Check, ShieldCheck,
  Power, PowerOff, Edit3, Key,
} from 'lucide-react'
import { useAuth } from '../../context/AuthContext'
import { showToast as toast } from '../../utils/toast'
import { api, ApiError } from '../../utils/api'
import AdminLayout from '../../components/admin/AdminLayout'
import type { MerchantRecord } from '../../components/dashboard/types'

export default function MerchantsPage() {
  const navigate = useNavigate()
  const { logout } = useAuth()

  const [merchants, setMerchants] = useState<MerchantRecord[]>([])
  const [refreshing, setRefreshing] = useState(false)
  const [copiedKey, setCopiedKey] = useState<string | null>(null)

  // Create dialog
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')
  const [createSubmitting, setCreateSubmitting] = useState(false)

  // Edit dialog
  const [editTarget, setEditTarget] = useState<MerchantRecord | null>(null)
  const [editName, setEditName] = useState('')
  const [editSubmitting, setEditSubmitting] = useState(false)

  // New secret display
  const [newSecret, setNewSecret] = useState<{ appKey: string; appSecret: string } | null>(null)

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    try {
      const data = await api.get<MerchantRecord[]>('/admin/merchants')
      if (Array.isArray(data)) setMerchants(data)
      if (isRefresh) toast.success('商户数据已刷新')
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        logout()
        navigate('/login')
      } else {
        toast.error('加载失败: ' + (err instanceof Error ? err.message : '网络异常'))
      }
    } finally {
      if (isRefresh) setRefreshing(false)
    }
  }, [logout, navigate])

  useEffect(() => {
    const initialLoad = setTimeout(load, 0)
    return () => clearTimeout(initialLoad)
  }, [load])

  const handleRefresh = () => load(true)

  const handleCopy = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(label)
    toast.success(`${label} 已复制`)
    setTimeout(() => setCopiedKey(null), 2000)
  }

  // Create merchant
  const handleCreate = async () => {
    if (!newName.trim() || newName.trim().length < 2) {
      toast.error('商户名称至少 2 个字符')
      return
    }
    setCreateSubmitting(true)
    try {
      const result = await api.post<MerchantRecord>('/admin/merchants', { name: newName.trim() })
      if (result) {
        toast.success(`商户 "${result.name}" 创建成功`)
        setNewName('')
        setShowCreate(false)
        // Show the secret once
        if (result.appSecret && !result.appSecret.includes('•')) {
          setNewSecret({ appKey: result.appKey, appSecret: result.appSecret })
        }
        load()
      }
    } catch (err: unknown) {
      toast.error('创建失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setCreateSubmitting(false)
    }
  }

  // Edit merchant
  const openEdit = (m: MerchantRecord) => {
    setEditTarget(m)
    setEditName(m.name)
  }

  const handleEdit = async () => {
    if (!editTarget) return
    if (!editName.trim() || editName.trim().length < 2) {
      toast.error('商户名称至少 2 个字符')
      return
    }
    setEditSubmitting(true)
    try {
      await api.patch(`/admin/merchants/${editTarget.id}`, { name: editName.trim() })
      toast.success('商户信息已更新')
      setEditTarget(null)
      load()
    } catch (err: unknown) {
      toast.error('更新失败: ' + (err instanceof Error ? err.message : '网络异常'))
    } finally {
      setEditSubmitting(false)
    }
  }

  // Toggle merchant
  const handleToggle = async (m: MerchantRecord) => {
    try {
      await api.post(`/admin/merchants/${m.id}/toggle`)
      toast.success(`商户 "${m.name}" 已${m.isActive ? '停用' : '启用'}`)
      load()
    } catch (err: unknown) {
      toast.error('操作失败: ' + (err instanceof Error ? err.message : '网络异常'))
    }
  }

  return (
    <AdminLayout
      title="商户管理"
      subtitle={`共 ${merchants.length} 个商户`}
      refreshing={refreshing}
      onRefresh={handleRefresh}
      actions={
        <Button
          onPress={() => setShowCreate(true)}
          className="px-4 py-2 rounded-xl bg-emerald-500 hover:bg-emerald-600 text-white text-xs font-semibold flex items-center gap-1.5 cursor-pointer h-9"
        >
          <Plus className="w-3.5 h-3.5" /> 新建商户
        </Button>
      }
    >
      {/* New secret display banner */}
      {newSecret && (
        <Card className="p-5 mb-6 border-emerald-500/30 bg-emerald-500/5">
          <Card.Content className="p-0">
            <div className="flex items-start gap-3">
              <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center shrink-0">
                <ShieldCheck className="w-5 h-5" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-semibold text-foreground mb-1">商户密钥已生成 — 请立即保存</div>
                <div className="text-xs text-muted mb-3">关闭后将无法再次查看完整密钥</div>
                <div className="space-y-2 font-mono text-xs">
                  <div className="flex items-center gap-2">
                    <span className="text-muted w-16 shrink-0">AppKey</span>
                    <code className="flex-1 p-2 rounded bg-surface text-foreground select-all">{newSecret.appKey}</code>
                    <Button size="sm" variant="ghost" onPress={() => handleCopy(newSecret.appKey, 'AppKey')} className="px-2 py-1 h-auto min-h-0 min-w-0 cursor-pointer text-emerald-500">
                      {copiedKey === 'AppKey' ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                    </Button>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-muted w-16 shrink-0">Secret</span>
                    <code className="flex-1 p-2 rounded bg-surface text-foreground select-all break-all">{newSecret.appSecret}</code>
                    <Button size="sm" variant="ghost" onPress={() => handleCopy(newSecret.appSecret, 'AppSecret')} className="px-2 py-1 h-auto min-h-0 min-w-0 cursor-pointer text-emerald-500">
                      {copiedKey === 'AppSecret' ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                    </Button>
                  </div>
                </div>
              </div>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => setNewSecret(null)}
                className="text-muted hover:text-foreground px-2 py-1 h-auto min-h-0 min-w-0 cursor-pointer"
              >
                关闭
              </Button>
            </div>
          </Card.Content>
        </Card>
      )}

      {/* Merchant list */}
      {merchants.length === 0 ? (
        <Card className="p-12">
          <Card.Content className="p-0 text-center">
            <Store className="w-12 h-12 text-muted mx-auto mb-3" />
            <div className="text-sm text-muted">暂无商户，点击右上角「新建商户」创建</div>
          </Card.Content>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
          {merchants.map(m => (
            <Card key={m.id} className="p-5">
              <Card.Content className="p-0">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className={`w-10 h-10 rounded-xl flex items-center justify-center ${m.isActive ? 'bg-emerald-500/10 text-emerald-500' : 'bg-zinc-500/10 text-zinc-500'}`}>
                      <Store className="w-5 h-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-foreground">{m.name}</span>
                        <Chip
                          variant="soft"
                          size="sm"
                          className={`text-[10px] font-semibold border-none ${m.isActive ? 'bg-emerald-500/10 text-emerald-500' : 'bg-zinc-500/10 text-zinc-500'}`}
                        >
                          {m.isActive ? '运行中' : '已停用'}
                        </Chip>
                      </div>
                      <div className="text-[10px] text-muted mt-0.5">ID: {m.id}</div>
                    </div>
                  </div>
                </div>

                <div className="space-y-3 font-mono text-xs mb-4">
                  <div>
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="text-muted flex items-center gap-1"><Key className="w-3 h-3" /> AppKey</span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onPress={() => handleCopy(m.appKey, `AppKey-${m.id}`)}
                        className="text-[10px] text-emerald-500 flex items-center gap-1 px-1.5 py-0.5 h-auto min-w-0 min-h-0 cursor-pointer"
                      >
                        {copiedKey === `AppKey-${m.id}` ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                        复制
                      </Button>
                    </div>
                    <div className="p-2.5 rounded-lg bg-surface text-foreground text-xs select-all">{m.appKey}</div>
                  </div>
                  <div>
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="text-muted flex items-center gap-1"><Key className="w-3 h-3" /> AppSecret</span>
                    </div>
                    <div className="p-2.5 rounded-lg bg-surface text-muted text-xs">{m.appSecret}</div>
                  </div>
                </div>

                <div className="flex items-center gap-2 pt-4 border-t border-border">
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => openEdit(m)}
                    className="flex-1 px-3 py-2 text-xs text-muted hover:text-foreground hover:bg-surface rounded-lg flex items-center justify-center gap-1.5 cursor-pointer h-auto min-h-0"
                  >
                    <Edit3 className="w-3.5 h-3.5" /> 编辑
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => handleToggle(m)}
                    className={`flex-1 px-3 py-2 text-xs rounded-lg flex items-center justify-center gap-1.5 cursor-pointer h-auto min-h-0 ${
                      m.isActive
                        ? 'text-amber-500 hover:bg-amber-500/10'
                        : 'text-emerald-500 hover:bg-emerald-500/10'
                    }`}
                  >
                    {m.isActive ? <PowerOff className="w-3.5 h-3.5" /> : <Power className="w-3.5 h-3.5" />}
                    {m.isActive ? '停用' : '启用'}
                  </Button>
                </div>
              </Card.Content>
            </Card>
          ))}
        </div>
      )}

      {/* Create merchant modal */}
      <Modal>
        <Modal.Backdrop isOpen={showCreate} onOpenChange={open => !open && setShowCreate(false)} variant="blur">
          <Modal.Container size="sm">
            <Modal.Dialog className="rounded-xl border border-border bg-surface p-6 shadow-[0_18px_50px_rgba(0,0,0,0.12)]">
              <Modal.CloseTrigger />
              <Modal.Header className="pb-4">
                <Modal.Heading className="text-base font-semibold text-foreground flex items-center gap-2">
                  <Plus className="w-4 h-4 text-emerald-500" />
                  新建商户
                </Modal.Heading>
              </Modal.Header>
              <Modal.Body className="py-2 px-0">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted">商户名称</label>
                  <TextField aria-label="商户名称" fullWidth isDisabled={createSubmitting}>
                    <InputGroup className="w-full rounded-xl bg-surface border border-border focus-within:border-emerald-500 transition-colors">
                      <InputGroup.Input
                        type="text"
                        value={newName}
                        onChange={e => setNewName(e.target.value)}
                        placeholder="例如：XX科技有限公司"
                        className="w-full px-3 py-2.5 bg-transparent text-sm text-foreground focus:outline-none"
                        onKeyDown={e => e.key === 'Enter' && handleCreate()}
                      />
                    </InputGroup>
                  </TextField>
                </div>
              </Modal.Body>
              <Modal.Footer className="flex items-center justify-end gap-2 pt-6 px-0 pb-0">
                <Button
                  variant="ghost"
                  onPress={() => setShowCreate(false)}
                  isDisabled={createSubmitting}
                  className="px-4 py-2 text-xs text-muted hover:bg-surface rounded-lg cursor-pointer"
                >
                  取消
                </Button>
                <Button
                  onPress={handleCreate}
                  isDisabled={createSubmitting}
                  className="px-4 py-2 text-xs bg-emerald-500 hover:bg-emerald-600 text-white rounded-lg font-medium disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
                >
                  {createSubmitting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Plus className="w-3.5 h-3.5" />}
                  {createSubmitting ? '创建中...' : '确认创建'}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      {/* Edit merchant modal */}
      <Modal>
        <Modal.Backdrop isOpen={!!editTarget} onOpenChange={open => !open && setEditTarget(null)} variant="blur">
          <Modal.Container size="sm">
            <Modal.Dialog className="rounded-xl border border-border bg-surface p-6 shadow-[0_18px_50px_rgba(0,0,0,0.12)]">
              <Modal.CloseTrigger />
              <Modal.Header className="pb-4">
                <Modal.Heading className="text-base font-semibold text-foreground flex items-center gap-2">
                  <Edit3 className="w-4 h-4 text-blue-500" />
                  编辑商户
                </Modal.Heading>
              </Modal.Header>
              <Modal.Body className="py-2 px-0">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted">商户名称</label>
                  <TextField aria-label="商户名称" fullWidth isDisabled={editSubmitting}>
                    <InputGroup className="w-full rounded-xl bg-surface border border-border focus-within:border-emerald-500 transition-colors">
                      <InputGroup.Input
                        type="text"
                        value={editName}
                        onChange={e => setEditName(e.target.value)}
                        className="w-full px-3 py-2.5 bg-transparent text-sm text-foreground focus:outline-none"
                        onKeyDown={e => e.key === 'Enter' && handleEdit()}
                      />
                    </InputGroup>
                  </TextField>
                </div>
              </Modal.Body>
              <Modal.Footer className="flex items-center justify-end gap-2 pt-6 px-0 pb-0">
                <Button
                  variant="ghost"
                  onPress={() => setEditTarget(null)}
                  isDisabled={editSubmitting}
                  className="px-4 py-2 text-xs text-muted hover:bg-surface rounded-lg cursor-pointer"
                >
                  取消
                </Button>
                <Button
                  onPress={handleEdit}
                  isDisabled={editSubmitting}
                  className="px-4 py-2 text-xs bg-blue-500 hover:bg-blue-600 text-white rounded-lg font-medium disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
                >
                  {editSubmitting ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Edit3 className="w-3.5 h-3.5" />}
                  {editSubmitting ? '保存中...' : '保存'}
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </AdminLayout>
  )
}
