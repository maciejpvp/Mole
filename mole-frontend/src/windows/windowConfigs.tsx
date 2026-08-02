import type { WindowConfig } from '../types/window'
import { AdminUsersWindow } from './AdminUsersWindow'
import { CreateTunnelWindow } from './CreateTunnelWindow'
import { CardVerificationWindow } from './CardVerificationWindow'

export function createAdminUsersWindow(): WindowConfig {
	return {
		id: 'admin_users',
		title: 'Users',
		layout: { x: 0.04, y: 0.12, width: 0.92, height: 0.7 },
		children: <AdminUsersWindow />,
		showCloseBtn: true,
		defaultOpen: true,
	}
}

export function createCreateTunnelWindow(): WindowConfig {
	return {
		id: 'create_tunnel',
		title: 'Create Tunnel',
		layout: { x: 0.35, y: 0.25, width: 0.3, height: 0.35 },
		showCloseBtn: true,
		children: <CreateTunnelWindow />,
		defaultOpen: false,
	}
}

export function createCardVerificationWindow(): WindowConfig {
	return {
		id: 'card_verification',
		title: 'Card Verification',
		layout: { x: 0.32, y: 0.2, width: 0.36, height: 0.42 },
		children: <CardVerificationWindow />,
		showCloseBtn: true,
		defaultOpen: true,
	}
}
