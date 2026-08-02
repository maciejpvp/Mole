import { useEffect } from 'react'
import type { UserProfile } from '../lib/auth'
import { useWindowManager } from './useWindowManager'
import { AuthWindow } from '../windows/AuthWindow'
import { LimitsWindow } from '../windows/LimitsWindow'
import { TunnelsWindow } from '../windows/TunnelsWindow'
import { AdminDashboardWindow } from '../windows/AdminDashboardWindow'

function registerAuthWindow(addWindow: ReturnType<typeof useWindowManager>['addWindow']) {
	addWindow({
		id: 'auth',
		title: 'Auth',
		layout: { x: 0.3, y: 0.13, width: 0.35, height: 0.35 },
		children: <AuthWindow />,
	}, { open: false })
}

function registerUserWindows(
	user: UserProfile,
	addWindow: ReturnType<typeof useWindowManager>['addWindow'],
) {
	addWindow({
		id: 'limits',
		title: 'Limits',
		layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
		children: <LimitsWindow user={user} />,
	}, { open: false })
	addWindow({
		id: 'tunnels',
		title: 'Tunnels',
		layout: { x: 0.05, y: 0.4, width: 0.9, height: 0.12 },
		children: <TunnelsWindow user={user} />,
	}, { open: false })
}

function registerAdminWindow(addWindow: ReturnType<typeof useWindowManager>['addWindow']) {
	addWindow({
		id: 'admin_dashboard',
		title: 'Admin Dashboard',
		layout: { x: 0.05, y: 0.15, width: 0.28, height: 0.2 },
		children: <AdminDashboardWindow />,
	}, { open: false })
}

function removeAuthenticatedWindows(removeWindow: ReturnType<typeof useWindowManager>['removeWindow']) {
	removeWindow('limits')
	removeWindow('tunnels')
	removeWindow('create_tunnel')
	removeWindow('admin_dashboard')
	removeWindow('admin_users')
	removeWindow('card_verification')
}

export function useAppWindows(user: UserProfile | undefined) {
	const windowManager = useWindowManager()
	const { addWindow, removeWindow } = windowManager

	useEffect(() => {
		registerAuthWindow(addWindow)
	}, [addWindow])

	useEffect(() => {
		if (!user) {
			removeAuthenticatedWindows(removeWindow)
			return
		}

		registerUserWindows(user, addWindow)
		if (user.is_admin) {
			registerAdminWindow(addWindow)
			return
		}

		removeWindow('admin_dashboard')
		removeWindow('admin_users')
	}, [addWindow, removeWindow, user])

	return windowManager
}
