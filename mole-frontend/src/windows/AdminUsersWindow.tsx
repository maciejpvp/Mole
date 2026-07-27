import { useState } from 'react'
import { useAdminUsers } from '../hooks/useAdminUsers'
import type { AdminUsersQuery } from '../lib/api'

const pageSizes = [25, 50, 100]
const sortOptions: Array<{ value: AdminUsersQuery['sort']; label: string }> = [
	{ value: 'transfer', label: 'Transfer used' },
	{ value: 'minutes', label: 'Minutes used' },
	{ value: 'username', label: 'Username' },
	{ value: 'created_at', label: 'Created' },
]

function formatBytes(bytes: number) {
	if (!Number.isFinite(bytes)) return '—'
	if (bytes < 1024) return `${bytes} B`
	if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
	if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`
	return `${(bytes / 1024 ** 3).toFixed(2)} GB`
}

function formatDate(value: string | null) {
	if (!value) return '—'
	return new Date(value).toLocaleString()
}

function errorMessage(error: unknown) {
	return error instanceof Error ? error.message : 'Unable to load users'
}

export function AdminUsersWindow() {
	const [searchInput, setSearchInput] = useState('')
	const [query, setQuery] = useState<AdminUsersQuery>({
		limit: 50,
		sort: 'transfer',
		direction: 'desc',
	})
	const [cursorHistory, setCursorHistory] = useState<string[]>([])

	const usersQuery = useAdminUsers(query, true)
	const page = usersQuery.data?.data

	const applySearch = () => {
		setCursorHistory([])
		setQuery((current) => ({ ...current, search: searchInput.trim() || undefined, cursor: undefined }))
	}

	const changeSort = (sort: AdminUsersQuery['sort']) => {
		setCursorHistory([])
		setQuery((current) => ({
			...current,
			sort,
			direction: current.sort === sort && current.direction === 'desc' ? 'asc' : 'desc',
			cursor: undefined,
		}))
	}

	const changePageSize = (limit: number) => {
		setCursorHistory([])
		setQuery((current) => ({ ...current, limit, cursor: undefined }))
	}

	const nextPage = () => {
		if (!page?.next_cursor) return
		setCursorHistory((current) => [...current, query.cursor ?? ''])
		setQuery((current) => ({ ...current, cursor: page.next_cursor }))
	}

	const previousPage = () => {
		const history = [...cursorHistory]
		const previousCursor = history.pop()
		if (previousCursor === undefined) return
		setCursorHistory(history)
		setQuery((current) => ({ ...current, cursor: previousCursor || undefined }))
	}

	return (
		<div className="flex h-full min-h-0 flex-col font-mono text-[12px] leading-5 text-[#c5c5c5]">
			<form
				onSubmit={(event) => {
					event.preventDefault()
					applySearch()
				}}
				className="mb-2 flex flex-wrap items-center gap-2 border-b border-[#2b2f3a] pb-2"
			>
				<label className="text-[#569cd6]" htmlFor="admin-user-search">SEARCH</label>
				<input
					id="admin-user-search"
					value={searchInput}
					onChange={(event) => setSearchInput(event.target.value)}
					placeholder="username or email prefix"
					className="min-w-52 border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4] outline-none focus:border-[#569cd6]"
				/>
				<button type="submit" className="border border-[#404859] px-2 py-1 text-[#4ec9b0] hover:bg-[#2b2f3a]">[ filter ]</button>
				<label className="text-[#569cd6]" htmlFor="admin-user-sort">SORT</label>
				<select
					id="admin-user-sort"
					value={query.sort}
					onChange={(event) => changeSort(event.target.value as AdminUsersQuery['sort'])}
					className="border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4]"
				>
					{sortOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
				</select>
				<button type="button" onClick={() => changeSort(query.sort)} className="border border-[#404859] px-2 py-1 text-[#9cdcfe] hover:bg-[#2b2f3a]">
					[{query.direction === 'asc' ? ' ↑ asc ' : ' ↓ desc '}]
				</button>
				<label className="text-[#569cd6]" htmlFor="admin-user-limit">PAGE</label>
				<select
					id="admin-user-limit"
					value={query.limit}
					onChange={(event) => changePageSize(Number(event.target.value))}
					className="border border-[#404859] bg-[#0f1115] px-2 py-1 text-[#d4d4d4]"
				>
					{pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}
				</select>
			</form>

			{usersQuery.isLoading ? <div className="py-6 text-center text-[#6a9955]">// Loading users…</div> : null}
			{usersQuery.isError ? <div className="border border-[#f44747] p-2 text-[#f44747]">// {errorMessage(usersQuery.error)}</div> : null}
			{page ? (
				<div className="min-h-0 flex-1 overflow-auto border border-[#2b2f3a]">
					<table className="w-full min-w-[900px] border-collapse text-left">
						<thead className="sticky top-0 bg-[#1e222b] text-[#569cd6]">
							<tr>
								{['USERNAME', 'EMAIL', 'PLAN', 'ROLE', 'MINUTES', 'TRANSFER', 'LAST LOGIN', 'CREATED'].map((heading) => <th key={heading} className="border-b border-[#404859] px-2 py-1 font-normal">{heading}</th>)}
							</tr>
						</thead>
						<tbody>
							{page.users.map((user) => (
								<tr key={user.id} className="border-b border-[#20242d] hover:bg-[#1e222b]">
									<td className="px-2 py-1 text-[#dcdcaa]">{user.username}</td>
									<td className="px-2 py-1 text-[#b5cea8]">{user.email}</td>
									<td className="px-2 py-1 text-[#9cdcfe]">{user.plan}</td>
									<td className="px-2 py-1">{user.is_admin ? <span className="text-[#4ec9b0]">admin</span> : 'user'}</td>
									<td className="px-2 py-1 text-right">{user.monthly_minutes_used}</td>
									<td className="px-2 py-1 text-right">{formatBytes(user.monthly_transfer_bytes_used)}</td>
									<td className="whitespace-nowrap px-2 py-1">{formatDate(user.last_login_at)}</td>
									<td className="whitespace-nowrap px-2 py-1">{formatDate(user.created_at)}</td>
								</tr>
							))}
						</tbody>
					</table>
					{page.users.length === 0 ? <div className="p-6 text-center text-[#6a9955]">// No users found</div> : null}
				</div>
			) : null}

			<div className="mt-2 flex items-center justify-between border-t border-[#2b2f3a] pt-2">
				<span className="text-[#808080]">{query.cursor ? 'continuation page' : 'first page'} · {page?.users.length ?? 0} users</span>
				<div className="flex gap-2">
					<button type="button" onClick={previousPage} disabled={cursorHistory.length === 0} className="border border-[#404859] px-2 py-1 text-[#9cdcfe] disabled:opacity-40">[ previous ]</button>
					<button type="button" onClick={nextPage} disabled={!page?.next_cursor || usersQuery.isFetching} className="border border-[#404859] px-2 py-1 text-[#9cdcfe] disabled:opacity-40">[ next ]</button>
				</div>
			</div>
		</div>
	)
}
