import { useEffect, useMemo, useState } from 'react'
import type { AdminUser } from '../lib/api'
import { useChangeAdminUserPlan } from '../hooks/useChangeAdminUserPlan'
import { useSetAdminUserPermission } from '../hooks/useSetAdminUserPermission'
import { useSetAdminUserBanned } from '../hooks/useSetAdminUserBanned'
import { usePlans } from '../hooks/usePlans'
import { errorMessage, formatBytes, formatDate } from '../utils'
import type { WindowConfig } from '../types/window'

type AdminUserDetailsWindowProps = {
	user: AdminUser
}

export function createAdminUserDetailsWindow(user: AdminUser): WindowConfig {
	return {
		id: `admin_user_details_${user.id}`,
		title: user.username,
		layout: { x: 0.25, y: 0.2, width: 0.5, height: 0.55 },
		children: <AdminUserDetailsWindow user={user} />,
		showCloseBtn: true,
		defaultOpen: false,
	}
}

export function AdminUserDetailsWindow({ user }: AdminUserDetailsWindowProps) {
	const [account, setAccount] = useState(user)
	const plansQuery = usePlans()
	const changePlan = useChangeAdminUserPlan()
	const setAdminPermission = useSetAdminUserPermission()
	const setBanned = useSetAdminUserBanned()
	const currentPlan = useMemo(
		() => plansQuery.data?.find((plan) => plan.name === account.plan),
		[plansQuery.data, account.plan],
	)
	const [selectedPlanId, setSelectedPlanId] = useState<number | undefined>(currentPlan?.id)
	const [selectedIsAdmin, setSelectedIsAdmin] = useState(account.is_admin)
	const [selectedIsBanned, setSelectedIsBanned] = useState(account.is_banned)

	useEffect(() => {
		setSelectedPlanId(currentPlan?.id)
	}, [currentPlan?.id])

	const planChanged = selectedPlanId !== undefined && selectedPlanId !== currentPlan?.id
	const adminPermissionChanged = selectedIsAdmin !== account.is_admin
	const bannedChanged = selectedIsBanned !== account.is_banned
	const isSaving = changePlan.isPending || setAdminPermission.isPending || setBanned.isPending
	const hasChanges = planChanged || adminPermissionChanged || bannedChanged

	const saveAccount = async () => {
		if (!hasChanges || isSaving) return

		changePlan.reset()
		setAdminPermission.reset()
		setBanned.reset()

		try {
			if (planChanged && selectedPlanId !== undefined) {
				const updatedAccount = await changePlan.mutateAsync({ userId: account.id, planId: selectedPlanId })
				setAccount(updatedAccount)
			}
			if (adminPermissionChanged) {
				const updatedAccount = await setAdminPermission.mutateAsync({ userId: account.id, isAdmin: selectedIsAdmin })
				setAccount(updatedAccount)
			}
			if (bannedChanged) {
				const updatedAccount = await setBanned.mutateAsync({ userId: account.id, isBanned: selectedIsBanned })
				setAccount(updatedAccount)
			}
		} catch {
			// The mutation status below describes the failed request. Any earlier
			// successful change remains persisted and is reflected in local state.
		}
	}

	return (
		<div className="flex min-h-full flex-col space-y-3 font-mono text-[12px] leading-5 text-[#c5c5c5]">
			<div className="grid grid-cols-[minmax(120px,auto)_1fr] gap-x-4 gap-y-1 border-b border-[#2b2f3a] pb-3">
				<ReadonlyField label="USERNAME" value={account.username} />
				<ReadonlyField label="EMAIL" value={account.email} />
				<ReadonlyField label="TRANSFER" value={formatBytes(account.monthly_transfer_bytes_used)} />
				<ReadonlyField label="MINUTES" value={String(account.monthly_minutes_used)} />
				<ReadonlyField label="LAST LOGIN" value={formatDate(account.last_login_at)} />
				<ReadonlyField label="CREATED" value={formatDate(account.created_at)} />
			</div>

			<div className="space-y-2">
				<div className="grid grid-cols-[120px_1fr] items-center gap-2">
					<label className="text-[#569cd6]" htmlFor={`admin-user-plan-${account.id}`}>PLAN</label>
					<select
						id={`admin-user-plan-${account.id}`}
						value={selectedPlanId ?? ''}
						onChange={(event) => setSelectedPlanId(event.target.value ? Number(event.target.value) : undefined)}
						disabled={plansQuery.isLoading || plansQuery.isError || isSaving}
						className="border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4] disabled:opacity-50"
					>
						<option value="">{plansQuery.isLoading ? 'Loading plans…' : 'Select plan'}</option>
						{plansQuery.data?.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}
					</select>
				</div>
				{plansQuery.isError ? <StatusMessage tone="error">{errorMessage(plansQuery.error, 'Unable to load plans')}</StatusMessage> : null}
				{changePlan.isError ? <StatusMessage tone="error">{errorMessage(changePlan.error, 'Unable to change plan')}</StatusMessage> : null}
			</div>

			<div className="space-y-2 border-t border-[#2b2f3a] pt-3">
				<div className="grid grid-cols-[120px_1fr] items-center gap-2">
					<label className="text-[#569cd6]" htmlFor={`admin-user-banned-${account.id}`}>BANNED</label>
					<select
						id={`admin-user-banned-${account.id}`}
						value={selectedIsBanned ? 'yes' : 'no'}
						onChange={(event) => setSelectedIsBanned(event.target.value === 'yes')}
						disabled={isSaving}
						className="w-fit border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4] disabled:opacity-50"
					>
						<option value="yes">YES</option>
						<option value="no">NO</option>
					</select>
				</div>
				<div className="grid grid-cols-[120px_1fr] items-center gap-2">
					<label className="text-[#569cd6]" htmlFor={`admin-user-is-admin-${account.id}`}>IS ADMIN</label>
					<select
						id={`admin-user-is-admin-${account.id}`}
						value={selectedIsAdmin ? 'yes' : 'no'}
						onChange={(event) => setSelectedIsAdmin(event.target.value === 'yes')}
						disabled={isSaving}
						className="w-fit border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4] disabled:opacity-50"
					>
						<option value="yes">YES</option>
						<option value="no">NO</option>
					</select>
				</div>
				<button type="button" disabled className="border border-[#404859] px-2 py-1 text-[#808080] disabled:opacity-60">[ reset usage limits ]</button>
				<div className="text-[#808080]">// account actions are not available yet</div>
			</div>

			{setAdminPermission.isError ? <StatusMessage tone="error">{errorMessage(setAdminPermission.error, 'Unable to update administrator permission')}</StatusMessage> : null}
			{setBanned.isError ? <StatusMessage tone="error">{errorMessage(setBanned.error, 'Unable to update ban status')}</StatusMessage> : null}
			{!changePlan.isError && !setAdminPermission.isError && !setBanned.isError && (changePlan.isSuccess || setAdminPermission.isSuccess || setBanned.isSuccess) ? <StatusMessage tone="success">Account updated.</StatusMessage> : null}
			<div className="mt-auto flex justify-end pt-2">
				<button
					type="button"
					onClick={() => void saveAccount()}
					disabled={!hasChanges || isSaving}
					className="border border-[#404859] px-2 py-1 text-[#4ec9b0] hover:bg-[#2b2f3a] disabled:opacity-40"
				>
					{isSaving ? '[ saving… ]' : '[ save ]'}
				</button>
			</div>
		</div>
	)
}

function ReadonlyField({ label, value }: { label: string; value: string }) {
	return <><span className="text-[#569cd6]">{label}</span><span className="truncate text-[#d4d4d4]">{value}</span></>
}

function StatusMessage({ children, tone }: { children: string; tone: 'error' | 'success' }) {
	return <div className={tone === 'error' ? 'border border-[#f44747] p-2 text-[#f44747]' : 'border border-[#4ec9b0] p-2 text-[#4ec9b0]'}>{children}</div>
}
