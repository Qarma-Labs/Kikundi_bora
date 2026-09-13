import { createFileRoute, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  useMemberDashboardSummary,
  useGroupDashboardSummary,
  useKatibuDashboardSummary,
  useHazinaDashboardSummary,
} from "@/hooks/use-scoped-dashboard";
import { AppShell } from "@/components/AppShell";
import { LoanApprovalsCard } from "@/components/LoanApprovalsCard";
import { useAuth } from "@/lib/auth-provider";
import { roleMap, type User } from "@/api/types";
import { roleSubtitle } from "@/lib/roles";
import { requireAuth } from "@/lib/role-guards";
import { tzs } from "@/lib/format";
import { groupsApi, INTERVAL_LABELS } from "@/api/groups";
import { useSystemHealth } from "@/hooks/use-admin";
import {
  MemberAkibaCard,
  GroupBalanceCard,
  PersonalMemberStatsCard,
} from "@/components/DashboardCards";
import {
  Users,
  PiggyBank,
  Receipt,
  TrendingUp,
  AlertCircle,
  ShieldCheck,
  Wallet,
  ClipboardList,
  CheckCircle2,
  Loader2,
  Shield,
  Activity,
  Settings,
  CalendarDays,
  Gift,
  BadgeCheck,
} from "lucide-react";
import {
  useWelfareEvents,
  useConfirmWelfareReceipt,
} from "@/hooks/use-welfare";
import { useAppModal } from "@/components/AppModal";
import {
  DissolvedBanner,
  DissolutionVotingCard,
} from "@/components/DissolutionCard";
import { dissolutionApi } from "@/api/dissolution";

export const Route = createFileRoute("/dashibodi")({
  head: () => ({
    meta: [
      { title: "Dashibodi — Money Seeking" },
      {
        name: "description",
        content:
          "Muhtasari wa hali ya kikundi: wanachama, michango, mikopo na marejesho.",
      },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
  },
  component: Dashibodi,
});

function Dashibodi() {
  const { user } = useAuth();

  if (!user) return null;

  const jina = user.name.split(" ")[0] || "rafiki";
  const displayRole = roleMap[user.role] || "Mwanachama";

  return (
    <AppShell
      title={`Habari, ${jina}`}
      subtitle={roleSubtitle[displayRole] || ""}
    >
      <DashboardContent user={user} />
    </AppShell>
  );
}

function DashboardContent({ user }: { user: User }) {
  const displayRole = roleMap[user.role] || "Mwanachama";
  const needsGroup = ["Mwenyekiti", "Mweka Hazina", "Katibu"].includes(
    displayRole,
  );
  const { data: currentGroup } = useQuery({
    queryKey: ["groups", "current"],
    queryFn: groupsApi.current,
    enabled: needsGroup,
    staleTime: 5 * 60 * 1000,
  });
  const groupId = currentGroup?.data.id;

  return (
    <>
      <DissolvedBanner />
      <DissolutionBannerForDashboard groupId={groupId} />
      <div className="mb-5 flex items-center gap-2">
        <span className="chip bg-primary/10 text-primary">{displayRole}</span>
        <span className="text-xs text-muted-foreground">
          Mwonekano umetengenezwa kwa jukumu lako
        </span>
      </div>

      {/* BUG-2 fix: role-scoped "loans awaiting my approval" queue (leadership + bodi) */}
      <div className="mb-5">
        <LoanApprovalsCard />
      </div>

      {displayRole === "Mwenyekiti" && (
        <ChairmanView
          groupId={groupId}
          memberId={user.member_id || undefined}
        />
      )}
      {displayRole === "Mweka Hazina" && (
        <TreasurerView
          groupId={groupId}
          memberId={user.member_id || undefined}
        />
      )}
      {displayRole === "Katibu" && (
        <SecretaryView
          groupId={groupId}
          memberId={user.member_id || undefined}
        />
      )}
      {displayRole === "Mwanachama" && (
        <MemberView
          userName={user.name}
          memberId={user.member_id || null}
          memberCode={user.member_code || null}
          userPhone={user.phone}
        />
      )}
      {displayRole === "Msimamizi" && <AdminView />}
    </>
  );
}

// ---------- MWENYEKITI ----------
function ChairmanView({
  groupId,
  memberId,
}: {
  groupId?: string;
  memberId?: string;
}) {
  const { isLoading: groupLoading } = useGroupDashboardSummary(groupId);

  if (!groupId || groupLoading) return <LoadingSkeleton />;

  return (
    <>
      {/* Kikundi + Personal — same hero-surface / card-surface colours as dashibodi-old */}
      <div className="grid gap-5 lg:grid-cols-2">
        <GroupBalanceCard groupId={groupId!} />
        {memberId && <PersonalMemberStatsCard memberId={memberId} />}
      </div>
      <SectionTitle>Kazi zako kuu</SectionTitle>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <QuickAction to="/mikopo" icon={ShieldCheck} label="Idhinisha Mikopo" />
        <QuickAction to="/wanachama" icon={Users} label="Wanachama" />
        <QuickAction to="/ripoti" icon={TrendingUp} label="Tazama Ripoti" />
        <QuickAction to="/taarifa-za-marejesho" icon={Receipt} label="Taarifa Za Marejesho" />
      </div>
    </>
  );
}

// ---------- MWEKA HAZINA ----------
function TreasurerView({
  groupId,
  memberId,
}: {
  groupId?: string;
  memberId?: string;
}) {
  const {
    data: hazinData,
    isLoading,
    error,
  } = useHazinaDashboardSummary(groupId);

  if (!groupId || isLoading) return <LoadingSkeleton />;

  if (error || !hazinData) {
    return (
      <div className="card-surface border border-destructive/50 bg-destructive/5">
        <div className="flex items-start gap-3">
          <AlertCircle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
          <div>
            <h3 className="font-semibold text-sm mb-1">Imeshinikana kupakia</h3>
            <p className="text-xs text-muted-foreground">
              Haijapatikana data ya hazina. Tafadhali jaribu tena.
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Kikundi + Personal — same design */}
      <div className="grid gap-5 lg:grid-cols-2">
        <GroupBalanceCard groupId={groupId!} />
        {memberId && <PersonalMemberStatsCard memberId={memberId} />}
      </div>

      <div className="mt-5">
        <HeroBalance
          label="Mapato ya Mwezi Huu"
          value={tzs(Number(hazinData.cash_in_this_period ?? 0))}
          stats={[
            ["Imechukuniwa", tzs(hazinData.cash_in_confirmed ?? 0)],
            [
              "Inasub.",
              `${tzs(hazinData.cash_in_pending ?? 0)} (${hazinData.cash_in_pending_count})`,
            ],
            ["Marejesho", tzs(hazinData.repayments_this_month ?? 0)],
            ["Salio", tzs(hazinData.available_balance ?? 0)],
          ]}
        />
      </div>
      <SectionTitle>Kazi zako za leo</SectionTitle>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
        <QuickAction to="/michango" icon={PiggyBank} label="Pokea Mchango" />
        <QuickAction to="/taarifa-za-marejesho" icon={Receipt} label="Pokea Marejesho" />
        <QuickAction to="/thibitisha-marejesho" icon={BadgeCheck} label="Thibitisha Marejesho" />
        <QuickAction to="/mikopo" icon={Wallet} label="Simamia Mikopo" />
      </div>
      {(hazinData.recent_disbursements ?? []).length > 0 && (
        <>
          <SectionTitle>Mikopo iliyopewa karibuni</SectionTitle>
          <div className="card-surface divide-y divide-border">
            {(hazinData.recent_disbursements ?? []).slice(0, 5).map((d) => (
              <div
                key={d.loan_id}
                className="flex items-center justify-between px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{d.full_name}</p>
                  <p className="text-xs text-muted-foreground">{d.member_no}</p>
                </div>
                <p className="shrink-0 text-sm font-semibold">
                  {tzs(Number(d.amount))}
                </p>
              </div>
            ))}
          </div>
        </>
      )}
    </>
  );
}

// ---------- KATIBU ----------
function SecretaryView({
  groupId,
  memberId,
}: {
  groupId?: string;
  memberId?: string;
}) {
  const {
    data: katibuData,
    isLoading,
    error,
  } = useKatibuDashboardSummary(groupId);

  if (!groupId || isLoading) return <LoadingSkeleton />;

  if (error || !katibuData) {
    return (
      <div className="card-surface border border-destructive/50 bg-destructive/5">
        <div className="flex items-start gap-3">
          <AlertCircle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
          <div>
            <h3 className="font-semibold text-sm mb-1">
              Imeshindikana kupakia
            </h3>
            <p className="text-xs text-muted-foreground">
              Haijapatikana data ya katibu. Tafadhali jaribu tena.
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Kikundi + Personal — same design */}
      <div className="grid gap-5 lg:grid-cols-2">
        <GroupBalanceCard groupId={groupId!} />
        {memberId && <PersonalMemberStatsCard memberId={memberId} />}
      </div>

      <div className="mt-5">
        <HeroBalance
          label="Wanachama wa Kikundi"
          value={String(katibuData.total_active_members ?? 0)}
          stats={[
            [
              "Wapya mwezi huu",
              String(katibuData.members_joined_this_month ?? 0),
            ],
            ["Waliondoka", String(katibuData.members_left_this_month ?? 0)],
            [
              "Inasub. idhinisho",
              String(katibuData.pending_user_approvals ?? 0),
            ],
            ["Walichelewa", String(katibuData.late_payments_count ?? 0)],
          ]}
        />
      </div>
      <SectionTitle>Kazi zako</SectionTitle>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
        <QuickAction
          to="/wanachama"
          icon={ClipboardList}
          label="Sajili Mwanachama"
        />
        <QuickAction to="/michango" icon={PiggyBank} label="Kumbukumbu" />
        <QuickAction to="/ripoti" icon={TrendingUp} label="Andaa Ripoti" />
      </div>
      {(katibuData.late_payments ?? []).length > 0 && (
        <>
          <SectionTitle>Wanachama walio chelezo</SectionTitle>
          <div className="card-surface divide-y divide-border">
            {(katibuData.late_payments ?? []).slice(0, 8).map((p, i) => (
              <div
                key={`${p.member_id}-${i}`}
                className="flex items-center justify-between px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{p.full_name}</p>
                  <p className="text-xs text-muted-foreground">
                    {p.member_no} • {p.period_label}
                  </p>
                </div>
                <span className="chip bg-destructive/30 text-destructive">
                  Chelezo
                </span>
              </div>
            ))}
          </div>
        </>
      )}
    </>
  );
}

// ---------- MWANACHAMA ----------
function MemberView({
  userName,
  memberId,
  memberCode,
  userPhone,
}: {
  userName: string;
  memberId: string | null;
  memberCode: string | null;
  userPhone: string;
}) {
  const { data: settingsData } = useQuery({
    queryKey: ["groups", "settings"],
    queryFn: () => groupsApi.current(),
    staleTime: 5 * 60 * 1000,
  });
  const fixedAmount =
    settingsData?.data?.fixed_contribution_amount != null
      ? Number(settingsData.data.fixed_contribution_amount)
      : null;
  const nextDue = settingsData?.next_due_date ?? null;
  // Cycle state: once the member's contribution for the current round is
  // confirmed, the "Mchango ujao" banner disappears; pending shows feedback.
  const cycleStatus = settingsData?.my_contribution?.status ?? "none";
  const hideBanner = cycleStatus === "confirmed";
  const isPendingCycle = cycleStatus === "pending";

  // Welfare payouts awaiting THIS member's receipt confirmation.
  const { data: welfareData } = useWelfareEvents({
    status: "COMPLETED",
    limit: 100,
  });
  const confirmWelfareReceipt = useConfirmWelfareReceipt();
  const { showModal } = useAppModal();
  const awaitingReceipt = (welfareData?.data ?? []).filter(
    (ev: any) =>
      !!ev.disbursed_at &&
      !ev.received_at &&
      !!memberId &&
      (ev.member?.id === memberId || ev.member_id === memberId),
  );

  const {
    data: memberData,
    isLoading,
    error,
  } = useMemberDashboardSummary(memberId || undefined);

  if (memberId && isLoading) return <LoadingSkeleton />;

  if (!memberId) {
    return (
      <div className="card-surface p-6 text-center">
        <AlertCircle className="mx-auto h-10 w-10 text-warning" />
        <h2 className="mt-3 font-display text-lg font-bold">
          Kamilisha wasifu wako
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Karibu <span className="font-semibold">{userName}</span> — kamilisha
          taarifa zako kwenye ukurasa wa wasifu ili uweze kuomba mikopo na
          kuona michango yako.
        </p>
        <Link
          to="/wasifu"
          className="mt-4 inline-flex items-center gap-2 rounded-xl bg-primary px-5 py-2.5 text-sm font-semibold text-primary-foreground"
        >
          Nenda kwenye Wasifu
        </Link>
      </div>
    );
  }

  if (error || !memberData) {
    return (
      <div className="card-surface border border-destructive/50 bg-destructive/5">
        <div className="flex items-start gap-3">
          <AlertCircle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
          <div>
            <h3 className="font-semibold text-sm mb-1">
              Imeshindikana kupakia
            </h3>
            <p className="text-xs text-muted-foreground">
              Haijapatikana data ya akiba yako. Tafadhali jaribu tena.
            </p>
          </div>
        </div>
      </div>
    );
  }

  const meMemberNo = memberData.member_no || memberCode;
  const mePhone = userPhone;

  return (
    <>
      {awaitingReceipt.length > 0 && (
        <div className="card-surface p-4 mb-4 border-l-4 border-l-amber-500 flex items-center gap-3">
          <Gift className="h-5 w-5 shrink-0 text-amber-600" />
          <div>
            <p className="text-sm font-semibold">
              Fedha za kijamii zimetolewa kwa niaba yako
            </p>
            <p className="text-xs text-muted-foreground">
              {awaitingReceipt.length === 1
                ? "Mfuko 1 unasubiri uthibitisho wako"
                : `Mifuko ${awaitingReceipt.length} inasubiri uthibitisho wako`}
            </p>
          </div>
          {/* BUG-4: inline "Nimepokea" confirmation — the beneficiary closes the
              loop right here; leadership sees the received status afterwards. */}
          {awaitingReceipt.length === 1 ? (
            <button
              data-testid="confirm-welfare-received"
              onClick={async () => {
                try {
                  await confirmWelfareReceipt.mutateAsync(
                    awaitingReceipt[0].id,
                  );
                  showModal({
                    title: "Imefanikiwa",
                    message: "Asante! Umethibitisha kupokea fedha.",
                    variant: "success",
                    primaryLabel: "Sawa",
                  });
                } catch (e) {
                  showModal({
                    title: "Hitilafu",
                    message: (e as Error).message,
                    variant: "error",
                    primaryLabel: "Sawa",
                  });
                }
              }}
              disabled={confirmWelfareReceipt.isPending}
              className="ml-auto shrink-0 rounded-lg bg-amber-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-amber-600 disabled:opacity-50"
            >
              Nimepokea
            </button>
          ) : (
            <Link
              to="/mfuko-kijamii"
              className="ml-auto shrink-0 rounded-lg bg-amber-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-amber-600"
            >
              Angalia
            </Link>
          )}
        </div>
      )}
      {(fixedAmount != null || nextDue) && !hideBanner && (
        <div
          className={`card-surface p-4 mb-4 border-l-4 flex items-center gap-3 ${isPendingCycle ? "border-l-warning" : "border-l-primary"}`}
        >
          <CalendarDays
            className={`h-5 w-5 shrink-0 ${isPendingCycle ? "text-warning" : "text-primary"}`}
          />
          <div>
            {isPendingCycle ? (
              <>
                <p className="text-sm font-semibold">
                  Mchango wa kipindi hiki umeshapokelewa ✓
                </p>
                <p className="text-xs text-muted-foreground">
                  Unasubiri uthibitisho wa Hazina
                  {nextDue
                    ? ` · kipindi kijacho kinaanza baada ya ${nextDue}`
                    : ""}
                </p>
              </>
            ) : (
              <>
                <p className="text-sm font-semibold">
                  Mchango ujao
                  {fixedAmount != null && (
                    <>: TZS {fixedAmount.toLocaleString()}</>
                  )}
                  {nextDue && <> · ifikapo {nextDue}</>}
                </p>
                <p className="text-xs text-muted-foreground">
                  Kipindi:{" "}
                  {settingsData
                    ? INTERVAL_LABELS[settingsData.data.contribution_interval]
                    : "—"}{" "}
                  · Wasilisha kupitia "Weka Mchango"
                </p>
              </>
            )}
          </div>
          {!isPendingCycle && (
            <Link
              to="/weka-mchango"
              className="ml-auto shrink-0 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90"
            >
              Weka Mchango
            </Link>
          )}
          <Link
            to="/deni-langu"
            className="ml-auto shrink-0 rounded-lg border border-border px-3 py-1.5 text-xs font-semibold hover:bg-muted"
          >
            Deni Langu
          </Link>
        </div>
      )}
      <HeroBalance
        label="Akiba Yangu"
        value={tzs(
          Number(
            memberData.available_savings ?? memberData.total_contributions ?? 0,
          ),
        )}
        stats={[
          ["Michango", String(memberData.contributions_count ?? 0)],
          ["Mikopo wazi", String(memberData.outstanding_loans_count ?? 0)],
          ["Deni bado", tzs(memberData.outstanding_loans_balance ?? 0)],
          ["Mikopo iliyofungwa", String(memberData.closed_loans_count ?? 0)],
        ]}
      />
      {Number(memberData.total_offsets_applied ?? 0) > 0 && (
        <div className="card-surface mt-3 border-l-4 border-l-amber-500 p-4">
          <p className="text-sm font-semibold">
            Akiba iliyotumika kulipa mkopo uliochelewa:{" "}
            {tzs(Number(memberData.total_offsets_applied))}
          </p>
          <p className="text-xs text-muted-foreground">
            Jumla ya michango:{" "}
            {tzs(Number(memberData.total_contributions ?? 0))} · Angalia
            historia hapa chini kwa maelezo.
          </p>
        </div>
      )}

      <div className="mt-5 grid gap-3 md:grid-cols-2">
        <div className="card-surface flex items-center gap-3 p-4">
          <CheckCircle2 className="h-8 w-8 text-success" />
          <div>
            <p className="text-sm font-semibold">
              {meMemberNo
                ? `Mwanachama #${meMemberNo}`
                : `Mwanachama #${memberId?.slice(0, 8)}`}
            </p>
            <p className="text-xs text-muted-foreground">
              Namba ya simu: {mePhone}
            </p>
          </div>
        </div>
        <div className="card-surface p-4">
          <p className="text-xs text-muted-foreground">Namba ya simu</p>
          <p className="font-display text-lg font-bold">{mePhone}</p>
          <p className="text-xs text-muted-foreground">{meMemberNo ?? "—"}</p>
        </div>
      </div>

      <SectionTitle>Michango Yangu</SectionTitle>
      {(memberData.recent_contributions ?? []).length === 0 ? (
        <div className="card-surface p-6 text-center text-sm text-muted-foreground">
          Hakuna michango iliyorekodiwa bado.
        </div>
      ) : (
        <div className="card-surface divide-y divide-border">
          {(memberData.recent_contributions ?? []).slice(0, 6).map((c, i) =>
            c.source === "loan_offset" ? (
              <div
                key={`${c.created_at}-${i}`}
                className="flex items-center justify-between gap-3 bg-amber-50 px-4 py-3"
              >
                <div>
                  <p className="text-sm font-medium">
                    Akiba yako imetumika kulipa mkopo uliochelewa —{" "}
                    {tzs(c.amount)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {c.period_label} · punguzo la akiba, si malipo ya kawaida
                  </p>
                </div>
                <span className="chip shrink-0 bg-amber-200 text-amber-800">
                  OFFSET
                </span>
              </div>
            ) : (
              <div
                key={`${c.created_at}-${i}`}
                className="flex items-center justify-between px-4 py-3"
              >
                <div>
                  <p className="text-sm font-medium">{tzs(c.amount)}</p>
                  <p className="text-xs text-muted-foreground">
                    {c.period_label}
                  </p>
                </div>
                <span className="chip bg-success/15 text-success">
                  {c.status}
                </span>
              </div>
            ),
          )}
        </div>
      )}
    </>
  );
}

// ---------- MSIMAMIZI ----------
function AdminView() {
  const { data: health, isLoading } = useSystemHealth();

  if (isLoading) return <LoadingSkeleton />;

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div className="card-surface flex items-center gap-4 p-5">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-primary/10 text-primary">
            <Users className="h-6 w-6" />
          </span>
          <div>
            <p className="text-2xl font-bold">{health?.total_users ?? 0}</p>
            <p className="text-xs text-muted-foreground">Watumiaji wote</p>
          </div>
        </div>
        <div className="card-surface flex items-center gap-4 p-5">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-amber-100 text-amber-700">
            <Activity className="h-6 w-6" />
          </span>
          <div>
            <p className="text-2xl font-bold">{health?.active_users ?? 0}</p>
            <p className="text-xs text-muted-foreground">Wanaotumia</p>
          </div>
        </div>
        <div className="card-surface flex items-center gap-4 p-5">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-destructive/10 text-destructive">
            <AlertCircle className="h-6 w-6" />
          </span>
          <div>
            <p className="text-2xl font-bold">{health?.pending_users ?? 0}</p>
            <p className="text-xs text-muted-foreground">Wanaosubiri</p>
          </div>
        </div>
        <div className="card-surface flex items-center gap-4 p-5">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-success/15 text-success">
            <Users className="h-6 w-6" />
          </span>
          <div>
            <p className="text-2xl font-bold">{health?.total_members ?? 0}</p>
            <p className="text-xs text-muted-foreground">Wanachama</p>
          </div>
        </div>
        <div className="card-surface flex items-center gap-4 p-5">
          <span className="grid h-12 w-12 place-items-center rounded-xl bg-amber-100 text-amber-700">
            <Activity className="h-6 w-6" />
          </span>
          <div>
            <p className="text-2xl font-bold">
              {health?.recent_logins_24h ?? 0}
            </p>
            <p className="text-xs text-muted-foreground">Waliingia leo</p>
          </div>
        </div>
      </div>

      <SectionTitle>Haraka</SectionTitle>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <QuickAction to="/admin" icon={Settings} label="Simamia Watumiaji" />
        <QuickAction to="/admin-logs" icon={Activity} label="Kumbukumbu" />
        <QuickAction to="/wanachama" icon={Users} label="Wanachama" />
        <QuickAction to="/wasifu" icon={Shield} label="Wasifu Wangu" />
      </div>
    </>
  );
}

// ---------- shared bits ----------
function HeroBalance({
  label,
  value,
  stats,
}: {
  label: string;
  value: string;
  stats: [string, string][];
}) {
  return (
    <section className="hero-surface px-5 py-6 lg:px-7 lg:py-8">
      <p className="text-xs font-medium uppercase tracking-wider text-primary-foreground/80">
        {label}
      </p>
      <p className="mt-2 font-display text-4xl font-extrabold lg:text-5xl">
        {value}
      </p>
      <div className="mt-5 grid grid-cols-2 gap-3 text-sm lg:grid-cols-4">
        {stats.map(([k, v]) => (
          <div key={k} className="rounded-xl bg-white/15 px-3 py-2.5">
            <p className="text-xs text-primary-foreground/80">{k}</p>
            <p className="font-semibold">{v}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-3 mt-7 font-display text-base font-semibold">
      {children}
    </h2>
  );
}

function QuickAction({
  to,
  icon: Icon,
  label,
}: {
  to: string;
  icon: any;
  label: string;
}) {
  return (
    <Link
      to={to}
      className="card-surface flex items-center gap-3 p-3.5 transition-colors hover:border-primary/40"
    >
      <span className="grid h-10 w-10 place-items-center rounded-xl bg-primary text-primary-foreground">
        <Icon className="h-5 w-5" strokeWidth={2.25} />
      </span>
      <span className="text-sm font-semibold leading-tight">{label}</span>
    </Link>
  );
}

function DissolutionBannerForDashboard({ groupId }: { groupId?: string }) {
  const { data } = useQuery({
    queryKey: ["dissolution", "list", groupId],
    queryFn: () => dissolutionApi.listByGroup(groupId!),
    enabled: !!groupId,
  });
  const open = data?.data?.find((p) => p.status === "voting_open");
  if (!open) return null;
  return (
    <div className="mb-4">
      <DissolutionVotingCard proposalId={open.id} />
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="flex justify-center py-12">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  );
}
