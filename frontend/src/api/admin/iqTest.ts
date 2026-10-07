/**
 * Admin IQ Test API endpoints
 * Question banks, scheduled plans and scored runs for account IQ/quality testing.
 */

import { apiClient } from '../client'
import type {
  IQTestBank,
  IQTestPlan,
  IQTestRun,
  CreateIQTestBankRequest,
  UpdateIQTestBankRequest,
  CreateIQTestPlanRequest,
  UpdateIQTestPlanRequest,
  RunIQTestRequest
} from '@/types'

export interface IQTestRunQuery {
  bank_id?: number
  account_id?: number
  plan_id?: number
  limit?: number
}

export async function listBanks(): Promise<IQTestBank[]> {
  const { data } = await apiClient.get<IQTestBank[]>('/admin/iq-test/banks')
  return data ?? []
}

export async function getBank(id: number): Promise<IQTestBank> {
  const { data } = await apiClient.get<IQTestBank>(`/admin/iq-test/banks/${id}`)
  return data
}

export async function createBank(req: CreateIQTestBankRequest): Promise<IQTestBank> {
  const { data } = await apiClient.post<IQTestBank>('/admin/iq-test/banks', req)
  return data
}

export async function updateBank(id: number, req: UpdateIQTestBankRequest): Promise<IQTestBank> {
  const { data } = await apiClient.put<IQTestBank>(`/admin/iq-test/banks/${id}`, req)
  return data
}

export async function deleteBank(id: number): Promise<void> {
  await apiClient.delete(`/admin/iq-test/banks/${id}`)
}

export async function listPlansByBank(bankId: number): Promise<IQTestPlan[]> {
  const { data } = await apiClient.get<IQTestPlan[]>('/admin/iq-test/plans', {
    params: { bank_id: bankId }
  })
  return data ?? []
}

export async function createPlan(req: CreateIQTestPlanRequest): Promise<IQTestPlan> {
  const { data } = await apiClient.post<IQTestPlan>('/admin/iq-test/plans', req)
  return data
}

export async function updatePlan(id: number, req: UpdateIQTestPlanRequest): Promise<IQTestPlan> {
  const { data } = await apiClient.put<IQTestPlan>(`/admin/iq-test/plans/${id}`, req)
  return data
}

export async function deletePlan(id: number): Promise<void> {
  await apiClient.delete(`/admin/iq-test/plans/${id}`)
}

export async function runPlanNow(id: number): Promise<IQTestRun> {
  const { data } = await apiClient.post<IQTestRun>(`/admin/iq-test/plans/${id}/run`)
  return data
}

export async function listRunsByPlan(planId: number, limit?: number): Promise<IQTestRun[]> {
  const { data } = await apiClient.get<IQTestRun[]>(`/admin/iq-test/plans/${planId}/runs`, {
    params: limit ? { limit } : undefined
  })
  return data ?? []
}

export async function listRuns(query: IQTestRunQuery = {}): Promise<IQTestRun[]> {
  const { data } = await apiClient.get<IQTestRun[]>('/admin/iq-test/runs', { params: query })
  return data ?? []
}

export async function runBank(req: RunIQTestRequest): Promise<IQTestRun> {
  const { data } = await apiClient.post<IQTestRun>('/admin/iq-test/runs', req)
  return data
}

export async function getRun(id: number): Promise<IQTestRun> {
  const { data } = await apiClient.get<IQTestRun>(`/admin/iq-test/runs/${id}`)
  return data
}

export async function listPlansByAccount(accountId: number): Promise<IQTestPlan[]> {
  const { data } = await apiClient.get<IQTestPlan[]>(`/admin/accounts/${accountId}/iq-test-plans`)
  return data ?? []
}

export async function runForAccount(
  accountId: number,
  req: { bank_id: number; question_id: number; model_id?: string }
): Promise<IQTestRun> {
  const { data } = await apiClient.post<IQTestRun>(`/admin/accounts/${accountId}/iq-test/run`, req)
  return data
}

export async function listRunsByAccount(accountId: number, limit?: number): Promise<IQTestRun[]> {
  const { data } = await apiClient.get<IQTestRun[]>(`/admin/accounts/${accountId}/iq-test/runs`, {
    params: limit ? { limit } : undefined
  })
  return data ?? []
}

export const iqTestAPI = {
  listBanks,
  getBank,
  createBank,
  updateBank,
  deleteBank,
  listPlansByBank,
  createPlan,
  updatePlan,
  deletePlan,
  runPlanNow,
  listRunsByPlan,
  listRuns,
  runBank,
  getRun,
  listPlansByAccount,
  runForAccount,
  listRunsByAccount
}

export default iqTestAPI
