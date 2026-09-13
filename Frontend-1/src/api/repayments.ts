import { api } from "./client";
import type {
  Repayment,
  RecordRepaymentRequest,
  SubmitRepaymentRequest,
  PendingRepayment,
  RepaymentResponse,
  PaginatedResponse,
} from "./types";

export const repaymentsApi = {
  list: (params?: {
    page?: number;
    limit?: number;
    loan_id?: string;
    member_id?: string;
  }) => {
    const q: Record<string, string> = {};
    if (params?.page) q.page = String(params.page);
    if (params?.limit) q.limit = String(params.limit);
    if (params?.loan_id) q.loan_id = String(params.loan_id);
    if (params?.member_id) q.member_id = String(params.member_id);
    return api.get<PaginatedResponse<Repayment>>("/repayments", q);
  },
  record: (data: RecordRepaymentRequest) =>
    api.post<{ message: string; data: RepaymentResponse }>(
      "/repayments",
      data
    ),

  /** Member self-service: submit against OWN disbursed loan (starts PENDING). */
  submit: (loanId: string, data: SubmitRepaymentRequest) =>
    api.post<{ message: string; data: Repayment }>(
      `/loans/${loanId}/repayments`,
      data
    ),

  /** Treasurer's pending queue (group-scoped). */
  pendingQueue: (groupId: string, status = "PENDING") =>
    api.get<{ data: PendingRepayment[]; total: number }>(
      `/groups/${groupId}/repayments?status=${status}`
    ),

  /** MWEKA HAZINA ONLY. */
  approve: (id: string) =>
    api.patch<{ message: string; data: Repayment }>(`/repayments/${id}/approve`),

  /** MWEKA HAZINA ONLY (reason required). */
  reject: (id: string, reason: string) =>
    api.patch<{ message: string; data: Repayment }>(`/repayments/${id}/reject`, {
      reason,
    }),
};
