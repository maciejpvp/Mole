import { useWindowContext } from '../hooks/useWindowContext'
import { createAdminUsersWindow } from './windowConfigs'

export function AdminDashboardWindow() {
	const { addWindow } = useWindowContext()

	return (
		<div className="font-mono text-[13px] leading-5 text-[#c5c5c5]">
			<div className="mb-3 border-b border-[#2b2f3a] pb-2 text-[#569cd6]">
				[=] Administrator tools
			</div>
			<button
				onClick={() => addWindow(createAdminUsersWindow())}
				className="border border-[#404859] bg-[#1e222b] px-3 py-2 text-[#4ec9b0] hover:border-[#569cd6] hover:bg-[#2b2f3a] hover:text-[#9cdcfe]"
			>
				[ ListUsers ]
			</button>
		</div>
	)
}
