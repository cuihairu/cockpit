import type { DriftCheckItem } from '@/types'

// Drift 页共享呈现常量（index.tsx 清单列与 DiffModal 标题共用）
export const KIND_LABEL: Record<DriftCheckItem['kind'], string> = {
  nginx: '反代站点',
  traefik: 'Traefik 站点',
  cron: '定时任务',
  stack: '应用部署',
}

export const KIND_COLOR: Record<DriftCheckItem['kind'], string> = {
  nginx: 'green',
  traefik: 'cyan',
  cron: 'blue',
  stack: 'purple',
}
