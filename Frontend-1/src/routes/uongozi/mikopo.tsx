import { createFileRoute, Link } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth-provider";
import { requireAuth } from "@/lib/role-guards";
import { AppShell } from "@/components/AppShell";
import {
  ShieldCheck,
  Check,
  X,
  Clock,
  Banknote,
  UserPlus,
  Users,
  ChevronRight,
  Wallet,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useIsCommitteeMember } from "@/hooks/use-loan-committee";
import { loanCommitteeApi } from "@/api/loan-committee";
import { useDisburseLoan } from "@/hooks/use-loans";
import { loansApi } from "@/api/loans";
import { useMembers } from "@/hooks/use-members";
import { tzs } from "@/lib/format";
import { useState } from "react";
import { useAppModal } from "@/components/AppModal";

export const Route = createFileRoute("/uongozi/mikopo")({
  beforeLoad: () => {
    requireAuth();
  },
  component: MikopoPage,
});

interface Loan {
  id: string;
  member_id: string;
  amount: string;
  approved_amount?: string;
  balance_remaining?: string;
  purpose?: string;
  due_date: string;
  status: string;
  rejection_reason?: string;
  applied_at: string;
  hazina_approved_at?: string;
  katibu_approved_at?: string;
  bodi_approved_at?: string;
  mwenyekiti_approved_at?: string;
  member?: { id: string; member_no: string; full_name: string; phone: string };
  /** BUG-2: computed by the backend — which sequential stage this loan awaits. */
  awaiting_role?: "hazina" | "katibu" | "bodi" | "mwenyekiti";
  my_turn?: boolean;
  term_days: number;
  interest_enabled: boolean;
  interest_type?: string;
  applicable_interest_rate: string | number;
  interest_amount: string | number;
  total_repayment: string | number;
}

function rateLabel(l: Loan): string {
  if (!l.interest_enabled) return "Hakuna Riba";
  return `${Number(l.applicable_interest_rate)}% / mwezi`;
}

function MikopoPage() {
  const { user, isLeadership } = useAuth();
  const qc = useQueryClient();
  const { showModal } = useAppModal();
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showBodiPopup, setShowBodiPopup] = useState(false);
  const [selectedMember, setSelectedMember] = useState("");

  const isHazina = user?.role === "treasurer";
  const isKatibu = user?.role === "secretary";
  const isMwenyekiti = user?.role === "chair";
  const { data: committeeCheck } = useIsCommitteeMember();
  const isBodi = user?.role === "member" && committeeCheck?.is_committee_member;

  if (!user || (!isLeadership && !isBodi)) {
    // BUG-2 fix: appointed bodi members (role=member) must reach their stage too.
    return (
      <AppShell title="Idhinisha Mikopo">
        <div className="card-surface p-12 text-center">
          <p className="text-muted-foreground">Huna ruhusa</p>
        </div>
      </AppShell>
    );
  }

  const { data: loans, isLoading, error: loansError } = useQuery({
    queryKey: ["uongozi", "mikopo", "pending"],
    // FIX: use the shared api client so the JWT is attached via
    // api.configure(getToken) instead of a hand-rolled fetch that sent
    // "Bearer null" when sessionStorage was empty (per-tab) and swallowed
    // the real 401 message ("Token ya ukaguzi haijapatikana").
    queryFn: async () => {
      const res = await loansApi.pendingApproval(false);
      return res.data as Loan[];
    },
  });

  const { data: membersData } = useMembers({ limit: 50 });

  // Fully-approved loans awaiting disbursement (Mweka Hazina's job).
  // FIX: visible to all leadership/bodi so Mwenyekiti sees where his
  // approved loan went ("haonekani" confusion). Only Hazina gets the button.
  const canSeeApproved = isHazina || isKatibu || isMwenyekiti || isBodi;
  const { data: approvedLoansData } = useQuery({
    queryKey: ["uongozi", "mikopo", "approved"],
    queryFn: () => loansApi.list({ status: "APPROVED", limit: 50 }),
    enabled: canSeeApproved,
  });
  const approvedLoans = (approvedLoansData?.data ?? []).filter(
    (l) => l.status === "APPROVED",
  );
  const disburseLoan = useDisburseLoan();

  const approveMutation = useMutation({
    mutationFn: async ({
      loanId,
      amount,
    }: {
      loanId: string;
      amount: string | number;
    }) => {
      const n = Number(amount);
      return loansApi.chainApprove(
        loanId,
        Number.isFinite(n) && n > 0 ? { approved_amount: n } : undefined,
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["uongozi", "mikopo"] });
    },
    onError: (err: Error) =>
      showModal({
        title: "Hitilafu",
        message: err.message,
        variant: "error",
        primaryLabel: "Sawa",
      }),
  });

  const appointMutation = useMutation({
    mutationFn: async (userId: string) => loanCommitteeApi.appointMember(userId),
    onSuccess: () => {
      setShowBodiPopup(false);
      setSelectedMember("");
      showModal({
        title: "Imefanikiwa",
        message: "Mwanachama ameongezwa kwenye Bodi ya Mikopo!",
        variant: "success",
        primaryLabel: "Sawa",
      });
    },
    onError: (err: Error) =>
      showModal({
        title: "Hitilafu",
        message: err.message,
        variant: "error",
        primaryLabel: "Sawa",
      }),
  });

  const handleApprove = (loan: Loan) => {
    showModal({
      title: "Thibitisha",
      message: `Idhinisha mkopo wa TZS ${Number(loan.amount).toLocaleString()}?`,
      variant: "warning",
      primaryLabel: "Thibitisha",
      secondaryLabel: "Ghairi",
      onPrimary: async () => {
        setActionLoading(loan.id);
        try {
          await approveMutation.mutateAsync({
            loanId: loan.id,
            amount: loan.amount,
          });
          showModal({
            title: "Imefanikiwa",
            message: "Umeidhinisha!",
            variant: "success",
            primaryLabel: "Sawa",
          });
        } catch {
          /* error handled by mutation */
        } finally {
          setActionLoading(null);
        }
      },
    });
  };

  // Parallel: kila mtu aidhinisha kwa muda wake — hakuna "anasubiri X".
  // Label inaonyesha waliomaliza / waliobaki.
  const getApprovalStep = (loan: Loan) => {
    const done = [
      loan.hazina_approved_at,
      loan.katibu_approved_at,
      loan.bodi_approved_at,
      loan.mwenyekiti_approved_at,
    ].filter(Boolean).length;
    const missing: string[] = [];
    if (!loan.hazina_approved_at) missing.push("Hazina");
    if (!loan.katibu_approved_at) missing.push("Katibu");
    if (!loan.bodi_approved_at) missing.push("Bodi");
    if (!loan.mwenyekiti_approved_at) missing.push("Mwenyekiti");
    if (missing.length === 0)
      return { step: 4, label: "Wote wameidhinisha", done: true };
    return { step: done, label: `Bado: ${missing.join(", ")} (${done}/4)`, done: false };
  };

  const canApprove = (loan: Loan) => {
    // Parallel — kila mtu asaini stage yake wakati wowote, bila kusubiriana.
    // Mkopo hautoki mpaka wote waidhinishe (backend ndiyo hufinalize).
    if (isHazina && !loan.hazina_approved_at) return true;
    if (isKatibu && !loan.katibu_approved_at) return true;
    if (isBodi && !loan.bodi_approved_at) return true;
    if (isMwenyekiti && !loan.mwenyekiti_approved_at) return true;
    return false;
  };

  const pendingLoans = loans ?? [];
  const availableMembers = (membersData?.data ?? []).filter(
    (m: any) => m.is_active,
  );

  return (
    <AppShell
      title="Idhinisha Mikopo"
      subtitle="Kila mtu aidhinisha kwa muda wake — mkopo hautoki mpaka wote (4/4) waidhinishe"
      action={
        /* BUG-3 fix: appointing to the loan board is MWENYEKITI's action —
         it used to be shown to katibu, which was wrong. */
        isMwenyekiti && (
          <button
            onClick={() => setShowBodiPopup(true)}
            className="inline-flex items-center gap-1.5 rounded-xl bg-accent px-3.5 py-2 text-sm font-semibold"
          >
            <UserPlus className="h-4 w-4" /> Unda Bodi Ya Mikopo
          </button>
        )
      }
    >
      {isLoading ? (
        <div className="card-surface animate-pulse p-6">
          <div className="h-4 w-1/3 rounded bg-muted" />
        </div>
      ) : loansError ? (
        <div className="card-surface p-12 text-center">
          <p className="font-semibold text-destructive">Imeshindikana kupakia mikopo</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {(loansError as Error).message}. Tafadhali ingia tena kama inaendelea.
          </p>
        </div>
      ) : pendingLoans.length === 0 ? (
        <div className="card-surface p-12 text-center">
          <ShieldCheck className="mx-auto h-12 w-12 text-muted-foreground/50" />
          <p className="mt-4 text-muted-foreground">
            Hakuna mikopo inayosubiri idhini
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {pendingLoans.map((loan) => {
            const step = getApprovalStep(loan);
            return (
              <div key={loan.id} className="card-surface p-5">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-2 flex-wrap">
                      <span className="chip bg-amber-100 text-amber-700 text-[10px] font-semibold px-2 py-0.5 rounded">
                        {loan.member?.member_no}
                      </span>
                      <span className="chip bg-blue-100 text-blue-700 text-[10px] font-semibold px-2 py-0.5 rounded inline-flex items-center gap-1">
                        <Clock className="h-3 w-3" /> {step.label}
                      </span>
                      {loan.my_turn && (
                        <span
                          data-testid="my-turn-chip"
                          className="chip bg-green-100 text-green-800 text-[10px] font-bold px-2 py-0.5 rounded inline-flex items-center gap-1"
                        >
                          <Check className="h-3 w-3" /> Zamu yako
                        </span>
                      )}
                    </div>
                    <h3 className="font-display text-lg font-semibold">
                      {loan.member?.full_name}
                    </h3>
                    <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                      <div>
                        <p className="text-muted-foreground">Kiasi</p>
                        <p className="font-semibold">
                          TZS {Number(loan.amount).toLocaleString()}
                        </p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">
                          Tarehe ya Kulipa
                        </p>
                        <p className="font-semibold">
                          {new Date(loan.due_date).toLocaleDateString("sw-TZ")}
                        </p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Muda (siku)</p>
                        <p className="font-semibold">{loan.term_days ?? "—"}</p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Riba</p>
                        <p className="font-semibold">{rateLabel(loan)}</p>
                      </div>
                    </div>
                    <p className="mt-2 text-sm">
                      <span className="text-muted-foreground">Jumla ya kurejesha: </span>
                      <span className="font-semibold">
                        TZS {Number(loan.total_repayment ?? loan.amount).toLocaleString()}
                      </span>
                    </p>

                    {/* Approval progress bar */}
                    <div className="mt-3 flex items-center gap-1 text-xs flex-wrap">
                      <span
                        className={`chip px-2 py-0.5 rounded-full ${loan.hazina_approved_at ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground"}`}
                      >
                        Hazina {loan.hazina_approved_at ? "✓" : ""}
                      </span>
                      <ChevronRight className="h-3 w-3 text-muted-foreground" />
                      <span
                        className={`chip px-2 py-0.5 rounded-full ${loan.katibu_approved_at ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground"}`}
                      >
                        Katibu {loan.katibu_approved_at ? "✓" : ""}
                      </span>
                      <ChevronRight className="h-3 w-3 text-muted-foreground" />
                      <span
                        className={`chip px-2 py-0.5 rounded-full ${loan.bodi_approved_at ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground"}`}
                      >
                        Bodi {loan.bodi_approved_at ? "✓" : ""}
                      </span>
                      <ChevronRight className="h-3 w-3 text-muted-foreground" />
                      <span
                        className={`chip px-2 py-0.5 rounded-full ${loan.mwenyekiti_approved_at ? "bg-success text-success-foreground" : "bg-muted text-muted-foreground"}`}
                      >
                        Mwenyekiti {loan.mwenyekiti_approved_at ? "✓" : ""}
                      </span>
                    </div>
                  </div>
                  <div className="flex flex-col gap-2">
                    {canApprove(loan) && (
                      <button
                        onClick={() => handleApprove(loan)}
                        disabled={actionLoading === loan.id}
                        className="inline-flex items-center gap-1.5 rounded-lg bg-green-600 px-4 py-2 text-sm font-semibold text-white hover:bg-green-700 disabled:opacity-50"
                      >
                        <Check className="h-4 w-4" />{" "}
                        {actionLoading === loan.id ? "..." : "Idhinisha"}
                      </button>
                    )}
                    <Link
                      to={`/ukaguzi-mkopo/${loan.id}`}
                      className="inline-flex items-center gap-1.5 rounded-lg border px-4 py-2 text-sm font-medium hover:bg-muted"
                    >
                      Angalia
                    </Link>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Approved loans awaiting disbursement — Mweka Hazina acts, others observe */}
      {canSeeApproved && approvedLoans.length > 0 && (
        <>
          <div className="mt-7 mb-3 flex items-center gap-2">
            <Banknote className="h-4 w-4 text-success" />
            <h2 className="font-display text-base font-semibold">
              Mikopo Zilizoidhinishwa — {isHazina ? "Toa Fedha" : "Inasubiri Mweka Hazina"} ({approvedLoans.length})
            </h2>
          </div>
          <div className="space-y-3">
            {approvedLoans.map((l) => (
              <div
                key={l.id}
                className="card-surface flex items-center justify-between gap-4 p-4"
              >
                <div className="min-w-0">
                  <p className="truncate font-semibold">
                    {l.member?.full_name ?? `Mwanachama #${l.member_id}`}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {tzs(Number(l.approved_amount ?? l.amount))} · Idhinishwa{" "}
                    {l.mwenyekiti_approved_at
                      ? new Date(l.mwenyekiti_approved_at).toLocaleDateString(
                          "sw-TZ",
                        )
                      : ""}
                  </p>
                </div>
                {isHazina ? (
                <button
                  onClick={() =>
                    showModal({
                      title: "Toa Fedha za Mkopo",
                      message: `Unatoa ${tzs(Number(l.approved_amount ?? l.amount))} kwa ${l.member?.full_name ?? "mwanachama"}. Hakikisha umemkabidhi fedha kabla ya kubofya.`,
                      variant: "warning",
                      primaryLabel: "Thibitisha Kutolea",
                      secondaryLabel: "Ghairi",
                      onPrimary: async () => {
                        try {
                          await disburseLoan.mutateAsync(l.id);
                          showModal({
                            title: "Imefanikiwa",
                            message:
                              "Fedha zimetolewa. Mkopo sasa ni Wazi (OUTSTANDING).",
                            variant: "success",
                            primaryLabel: "Sawa",
                          });
                        } catch {
                          /* handled by mutation */
                        }
                      },
                    })
                  }
                  disabled={disburseLoan.isPending}
                  className="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-success px-4 py-2 text-sm font-semibold text-white hover:bg-success/90 disabled:opacity-50"
                >
                  <Wallet className="h-4 w-4" /> Toa Fedha
                </button>
                ) : (
                  <span className="shrink-0 rounded-lg bg-muted px-3 py-2 text-xs font-semibold text-muted-foreground">
                    Inasubiri Mweka Hazina
                  </span>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      {/* Unda Bodi Ya Mikopo Popup */}
      {showBodiPopup && (
        <div
          className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 sm:items-center sm:p-4"
          onClick={() => setShowBodiPopup(false)}
        >
          <div
            className="w-full max-w-md rounded-t-3xl bg-card p-5 sm:rounded-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h3 className="font-display text-lg font-semibold flex items-center gap-2">
                <Users className="h-5 w-5" /> Unda Bodi Ya Mikopo
              </h3>
              <button
                onClick={() => setShowBodiPopup(false)}
                className="rounded-lg p-1.5 hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <p className="text-sm text-muted-foreground mb-4">
              Viongozi wote (Mwenyekiti, Katibu, Hazina) tayari ni wajumbe.
              Chagua mwanachama wa kawaida kama mwakilishi wa wanachama wengine.
            </p>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-muted-foreground">
                Chagua Mwanachama
              </span>
              <select
                value={selectedMember}
                onChange={(e) => setSelectedMember(e.target.value)}
                className="w-full rounded-xl border border-input bg-background px-3 py-2.5 text-sm"
              >
                <option value="">— Chagua mwanachama —</option>
                {availableMembers.map((m: any) => (
                  <option key={m.id} value={m.user_id || m.id}>
                    {m.full_name} ({m.member_no})
                  </option>
                ))}
              </select>
            </label>
            <button
              onClick={() =>
                selectedMember && appointMutation.mutate(selectedMember)
              }
              disabled={!selectedMember || appointMutation.isPending}
              className="mt-4 w-full rounded-xl bg-primary py-3 text-sm font-semibold text-primary-foreground disabled:opacity-50"
            >
              {appointMutation.isPending
                ? "Inaongeza..."
                : "Ongeza Kwenye Bodi"}
            </button>
          </div>
        </div>
      )}
    </AppShell>
  );
}
