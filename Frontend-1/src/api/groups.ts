import { api } from "./client";

// --- Types ---

export type ContributionInterval = "weekly" | "monthly" | "semi_annual" | "yearly";

export interface GroupInfo {
  id: string;
  name: string;
  location?: string | null;
  founded_year?: number | null;
  contribution_interval: ContributionInterval;
  contribution_due_date?: string | null;
  fixed_contribution_amount?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface GroupSettingProposal {
  id: string;
  group_id: string;
  contribution_interval: ContributionInterval;
  contribution_due_date?: string | null;
  fixed_contribution_amount?: string | null;
  status: "PENDING" | "APPROVED" | "REJECTED";
  proposed_by: string;
  approved_by?: string | null;
  rejection_reason?: string | null;
  created_at: string;
  reviewed_at?: string | null;
  proposer?: { id: string; name: string; role: string };
}

export interface MyContributionStatus {
  /** "none" = not yet contributed this cycle; "pending" = submitted, awaiting verification; "confirmed" = done */
  status: "none" | "pending" | "confirmed";
  period_due_date?: string;
}

export interface GroupSettingsResponse {
  data: GroupInfo;
  pending_proposal: GroupSettingProposal | null;
  next_due_date: string | null;
  /** Only present when the signed-in user holds a member row and the group has a due date configured. */
  my_contribution?: MyContributionStatus | null;
}

export interface ProposeSettingsRequest {
  contribution_interval: ContributionInterval;
  contribution_due_date: string;
  fixed_contribution_amount?: number;
}

export const INTERVAL_LABELS: Record<ContributionInterval, string> = {
  weekly: "Mara kwa wiki",
  monthly: "Mara kwa mwezi",
  semi_annual: "Mara mbili kwa mwaka",
  yearly: "Mara moja kwa mwaka",
};

// --- API ---

export const groupsApi = {
  /** Single-group deployment: resolves the group + settings + pending proposal. */
  current: () => api.get<GroupSettingsResponse>("/groups/current"),

  getSettings: (groupId: string) =>
    api.get<GroupSettingsResponse>(`/groups/${groupId}/contribution-settings`),

  /** Mwenyekiti (chair) only. */
  propose: (groupId: string, data: ProposeSettingsRequest) =>
    api.post<{ message: string; data: GroupSettingProposal }>(
      `/groups/${groupId}/contribution-settings/propose`,
      data
    ),

  /** Katibu (secretary) only — applies the pending proposal. */
  approve: (groupId: string) =>
    api.post<{ message: string; data: GroupInfo }>(
      `/groups/${groupId}/contribution-settings/approve`
    ),

  /** Katibu (secretary) only. */
  reject: (groupId: string, reason: string) =>
    api.post<{ message: string; data: GroupSettingProposal }>(
      `/groups/${groupId}/contribution-settings/reject`,
      { reason }
    ),

  /** Mwenyekiti (chair) + Msimamizi (admin) only — group profile metadata. */
  updateProfile: (groupId: string, data: { name?: string; location?: string; founded_year?: number }) =>
    api.patch<{ message: string; data: GroupInfo }>(
      `/groups/${groupId}/profile`,
      data
    ),
};

// --- Loan settings (riba + muda) ---

export interface LoanSettings {
  id: string;
  group_id: string;
  interest_enabled: boolean;
  /** Monthly percentage, e.g. "5.00" = 5% per month. Only meaningful when interest_enabled. */
  default_interest_rate: string;
  interest_type: "flat" | "reducing";
  min_term_days: number;
  max_term_days: number;
  updated_at?: string;
}

export interface LoanSettingsProposal {
  id: string;
  group_id: string;
  proposal_kind: string;
  loan_interest_enabled?: boolean | null;
  loan_default_interest_rate?: string | null;
  loan_interest_type?: string | null;
  loan_min_term_days?: number | null;
  loan_max_term_days?: number | null;
  status: "PENDING" | "APPROVED" | "REJECTED";
  proposed_by: string;
  rejection_reason?: string | null;
  created_at: string;
}

export interface LoanSettingsResponse {
  data: LoanSettings;
  pending_proposal: LoanSettingsProposal | null;
}

export interface ProposeLoanSettingsRequest {
  interest_enabled: boolean;
  default_interest_rate?: number;
  interest_type?: "flat" | "reducing";
  min_term_days: number;
  max_term_days: number;
}

export const loanSettingsApi = {
  /** Any authenticated user — the application form needs min/max + rate. */
  get: (groupId: string) =>
    api.get<LoanSettingsResponse>(`/groups/${groupId}/loan-settings`),

  /** Mwenyekiti (chair) only. */
  propose: (groupId: string, data: ProposeLoanSettingsRequest) =>
    api.post<{ message: string; data: LoanSettingsProposal }>(
      `/groups/${groupId}/loan-settings/propose`,
      data
    ),

  /** Katibu (secretary) only. */
  approve: (groupId: string) =>
    api.post<{ message: string; data: LoanSettings }>(
      `/groups/${groupId}/loan-settings/approve`
    ),

  /** Katibu (secretary) only. */
  reject: (groupId: string, reason: string) =>
    api.post<{ message: string; data: LoanSettingsProposal }>(
      `/groups/${groupId}/loan-settings/reject`,
      { reason }
    ),
};
