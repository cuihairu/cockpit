import React, { Component, ErrorInfo, ReactNode } from 'react'
import { Button, Result } from 'antd'

interface Props {
	children: ReactNode
}

interface State {
	hasError: boolean
	error: Error | null
	errorInfo: ErrorInfo | null
}

/**
 * Error Boundary 组件
 * 捕获子组件树中的 JavaScript 错误，记录错误日志，并显示备用 UI
 */
class ErrorBoundary extends Component<Props, State> {
	constructor(props: Props) {
		super(props)
		this.state = { hasError: false, error: null, errorInfo: null }
	}

	static getDerivedStateFromError(error: Error): Partial<State> {
		// 更新 state 使下一次渲染能够显示降级后的 UI
		return { hasError: true }
	}

	componentDidCatch(error: Error, errorInfo: ErrorInfo) {
		// 可以将错误日志上报给服务器
		console.error('Error Boundary caught an error:', error, errorInfo)
		this.setState({
			error,
			errorInfo,
		})
	}

	handleReset = () => {
		this.setState({ hasError: false, error: null, errorInfo: null })
		// 尝试重新加载页面
		window.location.reload()
	}

	render() {
		if (this.state.hasError) {
			// 可以渲染自定义的降级 UI
			return (
				<div style={{
					display: 'flex',
					justifyContent: 'center',
					alignItems: 'center',
					height: '100vh',
					padding: '24px',
				}}>
					<Result
						status="error"
						title="页面出现错误"
						subTitle="抱歉，页面遇到了一些问题。请尝试刷新页面或联系管理员。"
						extra={
							<Button type="primary" onClick={this.handleReset}>
								刷新页面
							</Button>
						}
					/>
				</div>
			)
		}

		return this.props.children
	}
}

export default ErrorBoundary
