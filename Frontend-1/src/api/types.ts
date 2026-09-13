export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
}

export interface MessageResponse {
  message: string;
  data?: unknown;
}

export interface LoginRequest {
  email: string; // email or phone
  password: string;
}

export interface RegisterRequest {
  name: string;
  email: string;
  phone: string;
  password: string;
  role: "chair" | "treasurer" | "secretary" | "member";
}

export interface AuthResponse {
  token: string;
  user: User;
  expires_at: string;
  first_login_required?: boolean;
}

export interface ChangePasswordRequest {
  old_password: string;
  new_password: string;
}

export interface UpdateProfileRequest {
  name?: string;
  phone?: string;
  avatar_url?: string;
  bio?: string;
}

export interface ResetPasswordRequest {
  email: string;
  new_password: string;
}

export type UserStatus = "PENDING" | "ACTIVE" | "REJECTED" | "SUSPENDED";

export type LeadershipRole = "MWENYEKITI" | "HAZINA" | "KATIBU";

export interface User {
  id: string;
  name: string;
  email?: string;
  phone: string;
  role: "chair" | "treasurer" | "secretary" | "member" | "admin";
  status: UserStatus;
  must_change_password: boolean;
  /** First-login app tour flag — true once the tour is completed/dismissed. */
  onboarding_seen?: boolean;
  avatar_url?: string;
  bio?: string;
  is_active: boolean;
  created_by?: string;
  approved_by?: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
  // Dual plane fields (from /me endpoint)
  member_id?: string;
  member_code?: string;
  leadership?: LeadershipRole[];
}

export type Jukumu = "Mwenyekiti" | "Mweka Hazina" | "Katibu" | "Mwanachama" | "Msimamizi";

export const roleMap: Record<string, Jukumu> = {
  chair: "Mwenyekiti",
  treasurer: "Mweka Hazina",
  secretary: "Katibu",
  member: "Mwanachama",
  admin: "Msimamizi",
};

export const reverseRoleMap: Record<Jukumu, string> = {
  Mwenyekiti: "chair",
  "Mweka Hazina": "treasurer",
  Katibu: "secretary",
  Mwanachama: "member",
  Msimamizi: "admin",
};

export interface Member {
  id: string;
  user_id?: string;
  member_no: string;
  full_name: string;
  phone: string;
  address?: string;
  joined_at: string;
  is_active: boolean;
  approval_status?: "pending" | "approved" | "rejected";
  registered_by: string;
  created_at: string;
  updated_at: string;
  registrar?: Pick<User, "id" | "name" | "email" | "role">;
}

export interface CreateMemberRequest {
  full_name: string;
  phone: string;
  address?: string;
  gender?: "MME" | "MKE";
  occupation?: string;
  email?: string;
  next_of_kin_name?: string;
  next_of_kin_phone?: string;
  photo_url?: string;
  joined_at: string;
  /** Mwenyekiti-only: retroactively charge main-cycle arrears from this cycle. Applies to michango kuu only, never social funds. */
  backdate_arrears?: boolean;
  /** Start cycle for backdating (YYYY-MM-DD or YYYY-MM). Sent only when backdate_arrears is true. */
  backdate_from_cycle?: string;
}

export interface UpdateMemberRequest {
  full_name?: string;
  phone?: string;
  address?: string;
  is_active?: boolean;
}

export interface Contribution {
  id: string;
  member_id: string;
  recorded_by: string;
  amount: number;
  month: string;
  paid_at: string;
  payment_method: "CASH" | "BANK" | "MOBILE_MONEY";
  reference_number?: string;
  receipt_url?: string;
  status: "PENDING" | "PAID" | "FAILED";
  confirmed_by?: string;
  notes?: string;
  created_at: string;
  member?: Pick<Member, "id" | "member_no" | "full_name" | "phone">;
  recorder?: Pick<User, "id" | "name" | "role">;
}

export interface CreateContributionRequest {
  member_id: string;
  amount: number;
  month: string;
  paid_at: string;
  payment_method: "CASH" | "BANK" | "MOBILE_MONEY";
  reference_number?: string;
  receipt_url?: string;
  notes?: string;
}

export interface EditContributionRequest {
  new_amount: number;
  reason: string;
}

export interface MonthlyReportRow {
  member_no: string;
  full_name: string;
  phone: string;
  amount_paid: number;
  paid_at?: string;
  status: "AMELIPA" | "HAJALIPA";
  notes?: string;
}

export interface MonthlyReportResponse {
  data: MonthlyReportRow[];
  month: string;
}

export type LoanStatus =
  | "PENDING"
  | "UNDER_REVIEW"
  | "APPROVED"
  | "OUTSTANDING"
  | "REJECTED"
  | "CLOSED";

export interface Loan {
  id: string;
  member_id: string;
  reviewed_by?: string;
  amount: number;
  approved_amount?: number;
  balance_remaining?: number;
  purpose?: string;
  due_date: string;
  status: LoanStatus;
  rejection_reason?: string;
  applied_at: string;
  reviewed_at?: string;
  updated_at: string;
  // Sequential approval trail (Hazina → Katibu → Bodi → Mwenyekiti)
  hazina_approved_at?: string;
  katibu_approved_at?: string;
  bodi_approved_at?: string;
  mwenyekiti_approved_at?: string;
  /** BUG-5: set when the borrower confirms receiving the disbursed loan. */
  borrower_confirmed_at?: string;
  // Loan term + interest snapshot (muda + riba)
  term_days: number;
  interest_enabled: boolean;
  interest_type?: string;
  applicable_interest_rate: number;
  interest_amount: number;
  total_repayment: number;
  member?: Pick<Member, "id" | "member_no" | "full_name" | "phone">;
  reviewer?: Pick<User, "id" | "name" | "role">;
}

export interface LoanInstallment {
  id: string;
  loan_id: string;
  number: number;
  due_date: string;
  principal_amount: number;
  interest_amount: number;
  total_amount: number;
  paid_amount: number;
  status: "PENDING" | "PAID";
}

export interface ApplyLoanRequest {
  member_id: string;
  amount: number;
  purpose?: string;
  due_date?: string;
  term_days?: number;
}

export interface ApproveLoanRequest {
  approved_amount: number;
}

export interface RejectLoanRequest {
  reason: string;
}

export interface LoanWithRepayments {
  data: Loan;
  repayments: Repayment[];
  installments?: LoanInstallment[];
}

export interface OutstandingReportRow {
  member_no: string;
  full_name: string;
  phone: string;
  loan_id: string;
  approved_amount: number;
  balance_remaining: number;
  amount_paid_so_far: number;
  due_date: string;
  urgency: "KAWAIDA" | "INAKARIBIA MUDA" | "IMEPITA MUDA";
  days_remaining: number;
}

export interface OutstandingReportResponse {
  data: OutstandingReportRow[];
}

export interface Repayment {
  id: string;
  loan_id: string;
  member_id: string;
  recorded_by: string;
  amount: number;
  balance_after: number;
  paid_at: string;
  payment_method: "CASH" | "BANK" | "MOBILE_MONEY";
  reference_number?: string;
  receipt_url?: string;
  notes?: string;
  created_at: string;
  status: "PENDING" | "CONFIRMED" | "REJECTED";
  submitted_by?: string;
  submitted_at?: string;
  proof_image_url?: string;
  proof_message?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  review_reason?: string;
  member?: Pick<Member, "id" | "member_no" | "full_name" | "phone">;
  recorder?: Pick<User, "id" | "name" | "role">;
}

export interface SubmitRepaymentRequest {
  amount: number;
  paid_at?: string;
  payment_method?: "CASH" | "BANK" | "MOBILE_MONEY";
  proof_image_url?: string;
  proof_message?: string;
  notes?: string;
}

export interface PendingRepayment {
  id: string;
  loan_id: string;
  member_id: string;
  amount: string | number;
  paid_at: string;
  payment_method: string;
  proof_image_url?: string;
  proof_message?: string;
  notes?: string;
  status: string;
  submitted_at?: string;
  created_at: string;
  member?: { id: string; member_no: string; full_name: string; phone: string };
  loan?: { id: string; amount?: string | number; approved_amount?: string | number; balance_remaining?: string | number; status?: string; due_date?: string };
}

export interface RecordRepaymentRequest {
  loan_id: string;
  amount: number;
  paid_at: string;
  payment_method: "CASH" | "BANK" | "MOBILE_MONEY";
  reference_number?: string;
  receipt_url?: string;
  notes?: string;
}

export interface RepaymentResponse {
  repayment_id: string;
  balance_after: number;
  loan_closed: boolean;
}

export interface DashboardSummary {
  total_active_members: number;
  total_contributions: number;
  total_loans_issued: number;
  total_repayments: number;
  total_outstanding_balance: number;
  count_outstanding_loans: number;
  count_pending_loans: number;
  members_paid_this_month: number;
  members_defaulted_this_month: number;
  total_contributions_this_month: number;
  total_repayments_this_month: number;
}

export type NotificationType =
  | "LOAN_REQUEST"
  | "LOAN_APPROVED"
  | "LOAN_REJECTED"
  | "LOAN_UNDER_REVIEW"
  | "COMMITTEE_APPOINTED"
  | "COMMITTEE_REMOVED"
  | "REPAYMENT"
  | "CONTRIBUTION"
  | "SYSTEM"
  | "WELFARE_CREATED"
  | "WELFARE_APPROVED"
  | "WELFARE_REJECTED"
  | "WELFARE_PAYMENT"
  | "WELFARE_COMPLETED"
  | "USER_CREATED"
  | "USER_APPROVED"
  | "USER_REJECTED"
  | "PASSWORD_SETUP";

export interface Notification {
  id: string;
  user_id: string;
  type: NotificationType;
  title: string;
  message: string;
  data?: Record<string, unknown>;
  read_at?: string;
  created_at: string;
}

export interface NotificationReadRequest {
  ids: string[];
}

export interface NotificationListResponse extends PaginatedResponse<Notification> {
  unread: number;
}

export type AuditAction =
  | "CREATE"
  | "UPDATE"
  | "DELETE"
  | "LOGIN"
  | "LOGOUT"
  | "APPROVE"
  | "REJECT"
  | "COMMITTEE_APPOINT"
  | "COMMITTEE_REMOVE"
  | "LOAN_REVIEW"
  | "LOAN_SUBMIT_REVIEW"
  | "CREATE_WELFARE_EVENT"
  | "APPROVE_WELFARE_EVENT"
  | "REJECT_WELFARE_EVENT"
  | "RECORD_WELFARE_PAYMENT"
  | "COMPLETE_WELFARE_EVENT"
  | "USER_CREATED"
  | "USER_APPROVED"
  | "USER_REJECTED"
  | "PASSWORD_SET"
  | "ADMIN_OVERRIDE";

export interface AuditLog {
  id: string;
  user_id?: string;
  action: AuditAction;
  table_name: string;
  record_id?: string;
  old_values?: Record<string, unknown>;
  new_values?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
  user?: Pick<User, "id" | "name" | "email" | "role">;
}

// User Management types

export interface CreateUserRequest {
  full_name: string;
  phone: string;
  role?: string;
}

export interface ApproveUserRequest {
  remarks?: string;
}

export interface RejectUserRequest {
  remarks?: string;
}

export interface FirstLoginSetupRequest {
  new_password: string;
  confirm_password: string;
}

export interface UserApproval {
  id: string;
  user_id: string;
  approved_by: string;
  status: "APPROVED" | "REJECTED";
  remarks?: string;
  approved_at: string;
  user?: Pick<User, "id" | "name" | "phone" | "role">;
  approver?: Pick<User, "id" | "name" | "role">;
}

// Admin types

export interface AdminOverrideRequest {
  action: "activate" | "deactivate" | "suspend";
  reason?: string;
}

export interface AdminResetPasswordRequest {
  new_password?: string;
}

export interface AdminLog {
  id: string;
  admin_id: string;
  action: string;
  target_user_id?: string;
  metadata?: string;
  ip_address: string;
  created_at: string;
  admin?: Pick<User, "id" | "name" | "role">;
  target_user?: Pick<User, "id" | "name" | "phone" | "role">;
}

export interface SystemHealth {
  total_users: number;
  pending_users: number;
  active_users: number;
  rejected_users: number;
  suspended_users: number;
  users_by_role: Array<{ role: string; count: number }>;
  recent_logins_24h: number;
  total_members: number;
  total_loans: number;
}

// Pending Actions types

export type PendingActionType = "CONTRIBUTION_EDIT" | "WELFARE_CREATE" | "LOAN_DISBURSE";
export type PendingActionStatus = "PENDING" | "APPROVED" | "REJECTED";

export interface PendingAction {
  id: string;
  action_type: PendingActionType;
  payload: Record<string, unknown>;
  requested_by: string;
  status: PendingActionStatus;
  approved_by?: string;
  remarks?: string;
  created_at: string;
  resolved_at?: string;
  requester?: Pick<User, "id" | "name" | "role">;
  approver?: Pick<User, "id" | "name" | "role">;
}

// User Position types

export type PositionType = "CHAIRPERSON" | "SECRETARY" | "TREASURER";

export interface UserPosition {
  id: string;
  user_id: string;
  position_type: PositionType;
  is_active: boolean;
  created_at: string;
}

// Role-scoped dashboard types (Task A backend integration)

export interface RecentContribution {
  id: string;
  source: "contribution" | "member_contribution" | "loan_offset";
  contribution_type: "AKIBA" | "MFUKO_WA_KIJAMII" | "OFFSET";
  period_label: string;
  amount: string;
  status: "PAID" | "CONFIRMED" | "PENDING_VERIFICATION" | "REJECTED" | "OFFSET_APPLIED";
  paid_at?: string;
  created_at: string;
}

export interface MemberDashboardSummary {
  member_id: string;
  member_no: string;
  full_name: string;
  total_contributions: string;
  contributions_count: number;
  total_offsets_applied: string;
  available_savings: string;
  welfare_contributions_total: string;
  welfare_contributions_count: number;
  pending_contributions_count: number;
  rejected_contributions_count: number;
  outstanding_loans_count: number;
  outstanding_loans_balance: string;
  closed_loans_count: number;
  recent_contributions: RecentContribution[];
}

export interface GroupDashboardSummary {
  group_id: string;
  group_name: string;
  total_active_members: number;
  total_contributions: string;
  total_repayments: string;
  total_disbursed: string;
  available_balance: string;
  outstanding_loans_count: number;
  outstanding_loans_balance: string;
  pending_loans_count: number;
  pending_contributions_count: number;
  contributions_this_period: string;
  contribution_interval: string;
  next_due_date?: string;
}

export interface LatePaymentRow {
  member_id: string;
  member_no: string;
  full_name: string;
  phone: string;
  period_label: string;
  expected_amount?: string;
}

export interface KatibuDashboardSummary {
  group_id: string;
  total_active_members: number;
  members_joined_this_month: number;
  members_left_this_month: number;
  pending_user_approvals: number;
  announcements_this_month: number;
  pending_contributions_count: number;
  current_period_label: string;
  next_due_date?: string;
  late_payments_count: number;
  late_payments: LatePaymentRow[];
}

export interface DisbursementRow {
  loan_id: string;
  member_no: string;
  full_name: string;
  amount: string;
  status: string;
  disbursed_at: string;
}

export interface HazinaDashboardSummary {
  group_id: string;
  cash_in_confirmed: string;
  cash_in_pending: string;
  cash_in_pending_count: number;
  cash_in_this_period: string;
  expected_this_period?: string;
  repayments_total: string;
  repayments_this_month: string;
  disbursements_total: string;
  disbursements_count: number;
  recent_disbursements: DisbursementRow[];
  available_balance: string;
}
