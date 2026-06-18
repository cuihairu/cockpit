export const getApiErrorMessage = (error: unknown, fallback: string): string => {
  if (typeof error !== 'object' || error === null) {
    return fallback
  }

  const response = 'response' in error ? error.response : undefined
  if (typeof response !== 'object' || response === null) {
    return fallback
  }

  const data = 'data' in response ? response.data : undefined
  if (typeof data !== 'object' || data === null || !('error' in data)) {
    return fallback
  }

  return typeof data.error === 'string' ? data.error : fallback
}
