// ========== 多云账号接入（cloud-provider.md 片2） ==========

export interface CloudAccount {
  id: string
  vendor: string
  alias: string
  credentialRef: string
  region?: string
  scopes: string[]
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface CloudAccountInput {
  vendor: string
  alias: string
  secret: string
  username?: string
  region?: string
  scopes?: string[]
  enabled?: boolean
}

export interface CloudInstance {
  id: string
  accountId: string
  nativeId: string
  name: string
  region: string
  zone?: string
  spec?: string
  status: string
  billingMode: string
  billingContinues: boolean
  rawVendorStatus?: string
  createdAt: string
  updatedAt: string
}

export interface CloudOperation {
  id: string
  kind: string
  native_id?: string
  vendor_request_id: string
  status: string
  error?: string
  updated_at: string
}

export interface CloudInstanceSpecInput {
  name: string
  region?: string
  zone?: string
  spec: string
  image_id: string
  security_group_id: string
  billing_mode?: string
}
