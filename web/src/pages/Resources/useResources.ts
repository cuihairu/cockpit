import { useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Certificate, ComputeInstance, Domain, Gateway, Service, Storage } from '@/types'
import { api } from '@/services/api'
import { useSettingsContext } from '@/contexts/useSettingsContext'

interface ResourcesData {
  computeInstances: ComputeInstance[]
  domains: Domain[]
  certificates: Certificate[]
  services: Service[]
  gateways: Gateway[]
  storages: Storage[]
}

interface ResourceState {
  loading: boolean
  computeInstances: ComputeInstance[]
  domains: Domain[]
  certificates: Certificate[]
  services: Service[]
  gateways: Gateway[]
  storages: Storage[]
  fetchAll: () => Promise<void>
}

const EMPTY_RESOURCES: ResourcesData = {
  computeInstances: [],
  domains: [],
  certificates: [],
  services: [],
  gateways: [],
  storages: [],
}

// 统一管理资源获取与状态，复用全局 QueryClient 缓存与刷新能力
export const useResources = (): ResourceState => {
  const { settings } = useSettingsContext()
  const { data = EMPTY_RESOURCES, isFetching, refetch } = useQuery({
    queryKey: ['resources'],
    queryFn: async (): Promise<ResourcesData> => {
      const [computeData, domainsData, certsData, servicesData, gatewaysData, storagesData] =
        await Promise.all([
          api.getComputeInstances(),
          api.getDomains(),
          api.getCertificates(),
          api.getServices(),
          api.getGateways(),
          api.getStorages(),
        ])

      return {
        computeInstances: computeData.data || [],
        domains: domainsData.data || [],
        certificates: certsData.data || [],
        services: servicesData.data || [],
        gateways: gatewaysData.data || [],
        storages: storagesData.data || [],
      }
    },
    refetchInterval: settings.refreshInterval * 1000,
  })

  const fetchAll = useCallback(async () => {
    await refetch()
  }, [refetch])

  return {
    loading: isFetching,
    computeInstances: data.computeInstances,
    domains: data.domains,
    certificates: data.certificates,
    services: data.services,
    gateways: data.gateways,
    storages: data.storages,
    fetchAll,
  }
}
