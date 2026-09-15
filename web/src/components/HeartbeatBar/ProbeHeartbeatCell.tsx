import { useQuery } from '@tanstack/react-query'
import { HeartbeatBar } from './index'
import { api } from '@/services/api'

// ProbeHeartbeatCell 拉取单个目标最近 N 轮探测历史并渲染心跳条
// （per-row 拉取，表格行数量级可接受；staleTime 减少翻页/重渲染时的重复请求）
export const ProbeHeartbeatCell: React.FC<{
  resourceType: string
  resourceId: string
  limit?: number
}> = ({ resourceType, resourceId, limit = 30 }) => {
  const { data, isLoading } = useQuery({
    queryKey: ['probe-history', resourceType, resourceId],
    queryFn: () => api.getProbeHistory(resourceType, resourceId, limit),
    staleTime: 60_000,
  })
  return <HeartbeatBar results={data?.results ?? []} loading={isLoading} />
}
