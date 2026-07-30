import { useEffect, useMemo, useState } from 'react'
import type { AdminUser } from '../lib/api'
import { useChangeAdminUserPlan } from '../hooks/useChangeAdminUserPlan'
import { useSetAdminUserPermission } from '../hooks/useSetAdminUserPermission'
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
	const currentPlan = useMemo(
		() => plansQuery.data?.find((plan) => plan.name === account.plan),
		[plansQuery.data, account.plan],
	)
	const [selectedPlanId, setSelectedPlanId] = useState<number | undefined>(currentPlan?.id)
	const [selectedIsAdmin, setSelectedIsAdmin] = useState(account.is_admin)

	useEffect(() => {
		setSelectedPlanId(currentPlan?.id)
	}, [currentPlan?.id])

	const planChanged = selectedPlanId !== undefined && selectedPlanId !== currentPlan?.id
	const adminPermissionChanged = selectedIsAdmin !== account.is_admin
	const isSaving = changePlan.isPending || setAdminPermission.isPending
	const hasChanges = planChanged || adminPermissionChanged

	const saveAccount = async () => {
		if (!hasChanges || isSaving) return

		changePlan.reset()
		setAdminPermission.reset()

		try {
			if (planChanged && selectedPlanId !== undefined) {
				const updatedAccount = await changePlan.mutateAsync({ userId: account.id, planId: selectedPlanId })
				setAccount(updatedAccount)
			}
			if (adminPermissionChanged) {
				const updatedAccount = await setAdminPermission.mutateAsync({ userId: account.id, isAdmin: selectedIsAdmin })
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
				<PlaceholderSelect id={`admin-user-banned-${account.id}`} label="BANNED" />
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
			{!changePlan.isError && !setAdminPermission.isError && (changePlan.isSuccess || setAdminPermission.isSuccess) ? <StatusMessage tone="success">Account updated.</StatusMessage> : null}
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

function PlaceholderSelect({ id, label }: { id: string; label: string }) {
	return (
		<div className="grid grid-cols-[120px_1fr] items-center gap-2">
			<label className="text-[#569cd6]" htmlFor={id}>{label}</label>
			<select id={id} defaultValue="no" disabled className="w-fit border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#808080] disabled:opacity-60">
				<option value="no">no</option>
				<option value="yes">yes</option>
			</select>
		</div>
	)
}

function StatusMessage({ children, tone }: { children: string; tone: 'error' | 'success' }) {
	return <div className={tone === 'error' ? 'border border-[#f44747] p-2 text-[#f44747]' : 'border border-[#4ec9b0] p-2 text-[#4ec9b0]'}>{children}</div>
}
