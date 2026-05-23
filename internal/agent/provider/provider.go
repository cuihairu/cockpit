package provider

// Provider 资源 Provider 接口
type Provider interface {
	// Type 返回 Provider 类型
	Type() string

	// Call 执行操作
	Call(action string, params map[string]interface{}) (interface{}, error)
}
