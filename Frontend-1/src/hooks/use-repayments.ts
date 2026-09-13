import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { repaymentsApi } from "@/api/repayments";
import type { SubmitRepaymentRequest } from "@/api/types";

export const repaymentKeys = {
  all: ["repayments"] as const,
  list: (params?: Record<string, unknown>) =>
    [...repaymentKeys.all, "list", params] as const,
  pendingQueue: (groupId: string | null) =>
    [...repaymentKeys.all, "pending-queue", groupId] as const,
};

export function useRepayments(params?: {
  page?: number;
  limit?: number;
  loan_id?: string;
  member_id?: string;
}) {
  return useQuery({
    queryKey: repaymentKeys.list(params as Record<string, unknown>),
    queryFn: () => repaymentsApi.list(params),
  });
}

/** Member self-service submission (starts PENDING — no balance movement). */
export function useSubmitRepayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ loanId, data }: { loanId: string; data: SubmitRepaymentRequest }) =>
      repaymentsApi.submit(loanId, data),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: repaymentKeys.all }),
  });
}

/** Treasurer's pending queue (group-scoped). */
export function usePendingRepayments(groupId: string | null) {
  return useQuery({
    queryKey: repaymentKeys.pendingQueue(groupId),
    queryFn: () => repaymentsApi.pendingQueue(groupId!),
    enabled: !!groupId,
  });
}

/** MWEKA HAZINA ONLY — approval moves the money. */
export function useApproveRepayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => repaymentsApi.approve(id),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: repaymentKeys.all }),
  });
}

/** MWEKA HAZINA ONLY (reason required). */
export function useRejectRepayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      repaymentsApi.reject(id, reason),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: repaymentKeys.all }),
  });
}
