import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import {
  useLoans,
  useApplyLoan,
  useConfirmLoanReceived,
} from "@/hooks/use-loans";
import { groupsApi, loanSettingsApi } from "@/api/groups";
import { loansApi } from "@/api/loans";
import { Field } from "@/components/Field";
import { tzs, tarehe } from "@/lib/format";
import { X, Send, Loader2, CheckCircle2 } from "lucide-react";
import { useAuth } from "@/lib/auth-provider";
import { blockAdminFromPage, requireAuth } from "@/lib/role-guards";
import { useAppModal } from "@/components/AppModal";
import { type LoanStatus } from "@/api/types";

export const Route = createFileRoute("/mikopo")({
  head: () => ({
    meta: [
      { title: "Mikopo — Money Seeking" },
      {
        name: "description",
        content: "Toa mikopo na fuatilia mizania ya wanachama.",
      },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
    blockAdminFromPage();
  },
  component: MikopoPage,
});

type Tab =
  | "PENDING"
  | "UNDER_REVIEW"
  | "APPROVED"
  | "OUTSTANDING"
  | "CLOSED"
  | "REJECTED";

const tabLabels: Record<Tab, string> = {
  PENDING: "Ombi",
  UNDER_REVIEW: "Inapitiwa",
  APPROVED: "Imeidhinishwa",
  OUTSTANDING: "Wazi",
  CLOSED: "Imefungwa",
  REJECTED: "Imekataliwa",
};

function MikopoPage() {
  const { user } = useAuth();
  const { showModal } = useAppModal();
  const confirmReceived = useConfirmLoanReceived();
  if (!user) return null;

  const [tab, setTab] = useState<Tab>("OUTSTANDING");
  const [openRequest, setOpenRequest] = useState(false);

  const { data: loansData, isLoading } = useLoans({ limit: 200 });
  const loans = loansData?.data ?? [];
  // Personal view — EVERY user (including leadership, dual plane) sees only
  // their own loans here. Group-wide management lives on /uongozi/mikopo.
  const myMemberId = user.member_id || null;
  const visible = myMemberId
    ? loans.filter((l) => l.member_id === myMemberId)
    : [];
  const list = visible.filter((l) => l.status === tab);
  const tabs: Tab[] = [
    "PENDING",
    "UNDER_REVIEW",
    "APPROVED",
    "OUTSTANDING",
    "CLOSED",
    "REJECTED",
  ];

  const jumlaWazi = visible
    .filter((l) => l.status === "OUTSTANDING")
    .reduce((s, l) => s + Number(l.balance_remaining ?? 0), 0);

  return (
    <AppShell
      title="Mikopo Yangu"
      subtitle="Omba na fuatilia mikopo yako"
      action={
        myMemberId ? (
          <button
            onClick={() => setOpenRequest(true)}
            className="inline-flex items-center gap-1.5 rounded-xl bg-primary px-3.5 py-2 text-sm font-semibold text-primary-foreground"
          >
            <Send className="h-4 w-4" /> Omba Mkopo
          </button>
        ) : null
      }
    >
      {!myMemberId && (
        <div className="card-surface mb-5 p-4">
          <p className="text-sm">Kamilisha taarifa zako kwenye wasifu ili uanze kuomba mikopo.</p>
          <Link
            to="/wasifu"
            className="mt-2 inline-block text-sm font-semibold text-primary"
          >
            Nenda kwenye Wasifu →
          </Link>
        </div>
      )}

      <div className="hero-surface px-5 py-5">
        <p className="text-xs text-primary-foreground/70">
          Salio la mikopo yangu
        </p>
        <p className="mt-1 font-display text-3xl font-extrabold">
          {tzs(jumlaWazi)}
        </p>
        <p className="mt-1 text-xs text-primary-foreground/70">
          {visible.filter((l) => l.status === "OUTSTANDING").length} mikopo wazi
        </p>
      </div>

      {isLoading && (
        <div className="mt-8 flex justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
        </div>
      )}

      <div className="mt-5 -mx-1 flex gap-1 overflow-x-auto pb-1">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`shrink-0 rounded-lg px-4 py-1.5 text-xs font-semibold ${tab === t ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"}`}
          >
            {tabLabels[t]} ({visible.filter((l) => l.status === t).length})
          </button>
        ))}
      </div>

      <div className="mt-3 space-y-2.5">
        {list.map((l) => {
          const bal = Number(
            l.balance_remaining ??
              (l.status === "APPROVED" ? (l.approved_amount ?? l.amount) : 0),
          );
          const pct = l.approved_amount
            ? Math.min(
                100,
                ((Number(l.approved_amount) - bal) /
                  Number(l.approved_amount)) *
                  100,
              )
            : 0;
          return (
            <div key={l.id} className="card-surface p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-semibold">
                    {l.purpose || "Mkopo wa kikundi"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Kilitumwa {tarehe(l.applied_at || l.due_date)}
                  </p>
                </div>
                <span className={`chip text-[10px] ${statusClass(l.status)}`}>
                  {/* BUG-5: disbursed-but-unconfirmed is a distinct visible state */}
                  {l.status === "OUTSTANDING" && !l.borrower_confirmed_at
                    ? "Imetolewa — Inasubiri Uthibitisho Wako"
                    : l.status === "OUTSTANDING" && l.borrower_confirmed_at
                      ? "Imetolewa — Imethibitishwa"
                      : (tabLabels[l.status as Tab] ?? l.status)}
                </span>
              </div>
              <div className="mt-3 grid grid-cols-3 gap-2 text-xs">
                <div>
                  <p className="text-muted-foreground">Kiasi</p>
                  <p className="font-semibold">{tzs(l.amount)}</p>
                </div>
                <div>
                  <p className="text-muted-foreground">
                    {l.status === "APPROVED" ? "Idhinishwa" : "Salio"}
                  </p>
                  <p
                    className={`font-semibold ${l.status !== "APPROVED" ? "text-warning" : ""}`}
                  >
                    {tzs(bal)}
                  </p>
                </div>
                <div>
                  <p className="text-muted-foreground">Mwisho</p>
                  <p className="font-semibold">{tarehe(l.due_date)}</p>
                </div>
                <div>
                  <p className="text-muted-foreground">Muda (siku)</p>
                  <p className="font-semibold">{l.term_days ?? "—"}</p>
                </div>
                <div>
                  <p className="text-muted-foreground">Riba</p>
                  <p className="font-semibold">
                    {l.interest_enabled ? `${Number(l.applicable_interest_rate)}%` : "Hakuna"}
                  </p>
                </div>
                <div>
                  <p className="text-muted-foreground">Jumla</p>
                  <p className="font-semibold">{tzs(Number(l.total_repayment ?? l.amount))}</p>
                </div>
              </div>
              {(l.status === "OUTSTANDING" || l.status === "CLOSED") && l.borrower_confirmed_at && (
                <LoanSchedule loanId={l.id} interestFree={!l.interest_enabled} />
              )}
              {l.status === "OUTSTANDING" && (
                <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full bg-success transition-all"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              )}
              {l.status === "OUTSTANDING" && !l.borrower_confirmed_at && (
                <button
                  data-testid="confirm-loan-received"
                  onClick={() =>
                    showModal({
                      title: "Thibitisha Umepokea Mkopo",
                      message: `Unathibitisha kuwa umepokea ${tzs(l.approved_amount ?? l.amount)} kutoka kwa Mweka Hazina?`,
                      variant: "warning",
                      primaryLabel: "Nimepokea",
                      secondaryLabel: "Ghairi",
                      onPrimary: async () => {
                        try {
                          await confirmReceived.mutateAsync(l.id);
                          showModal({
                            title: "Imefanikiwa",
                            message: "Asante! Umethibitisha kupokea mkopo.",
                            variant: "success",
                            primaryLabel: "Sawa",
                          });
                        } catch {
                          /* handled by mutation */
                        }
                      },
                    })
                  }
                  disabled={confirmReceived.isPending}
                  className="mt-3 inline-flex w-full items-center justify-center gap-1.5 rounded-lg bg-success px-4 py-2 text-sm font-semibold text-white hover:bg-success/90 disabled:opacity-50"
                >
                  <CheckCircle2 className="h-4 w-4" /> Thibitisha Umepokea Mkopo
                </button>
              )}
              {l.rejection_reason && (
                <p className="mt-2 text-xs text-destructive">
                  Sababu: {l.rejection_reason}
                </p>
              )}
              {l.status === "PENDING" && (
                <p className="mt-2 text-xs text-muted-foreground">
                  Ombi lako linapitiwa: kila kiongozi (Hazina, Katibu, Bodi,
                  Mwenyekiti) ataidhinisha kwa muda wake — mkopo hautoki mpaka
                  wote waidhinishe.
                </p>
              )}
            </div>
          );
        })}
        {list.length === 0 && !isLoading && (
          <div className="card-surface p-8 text-center text-sm text-muted-foreground">
            Hakuna mikopo katika hali hii.
          </div>
        )}
      </div>

      {openRequest && myMemberId && (
        <RequestForm
          memberId={myMemberId}
          onClose={() => setOpenRequest(false)}
        />
      )}
    </AppShell>
  );
}

/** Repayment schedule for a disbursed + confirmed loan (Mikopo Yangu). */
function LoanSchedule({ loanId, interestFree }: { loanId: string; interestFree: boolean }) {
  const [open, setOpen] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ["loans", "schedule", loanId],
    queryFn: () => loansApi.schedule(loanId),
    enabled: open,
  });
  const rows = data?.data ?? [];
  return (
    <div className="mt-3">
      <button
        onClick={() => setOpen(!open)}
        className="text-xs font-semibold text-primary"
      >
        {open ? "Ficha ratiba ya marejesho ▴" : "Ona ratiba ya marejesho ▾"}
      </button>
      {open && (
        <div className="mt-2 space-y-1.5">
          {isLoading && <p className="text-xs text-muted-foreground">Inapakia ratiba…</p>}
          {rows.map((r) => (
            <div key={r.id} className="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2 text-xs">
              <span className="font-semibold">Awamu {r.number}</span>
              <span className="text-muted-foreground">{tarehe(r.due_date)}</span>
              <span className="font-semibold">{tzs(Number(r.total_amount))}</span>
              <span className={`chip text-[10px] ${r.status === "PAID" ? "bg-success/15 text-success" : "bg-muted text-muted-foreground"}`}>
                {r.status === "PAID" ? "Imelipwa" : "Inasubiri"}
              </span>
            </div>
          ))}
          {rows.length > 0 && interestFree && (
            <p className="text-[11px] text-success">Huu ni mkopo usio na riba — kila awamu ni sehemu ya kiasi kikuu pekee.</p>
          )}
          {rows.length === 0 && !isLoading && (
            <p className="text-xs text-muted-foreground">Hakuna ratiba (mkopo wa zamani kabla ya mfumo wa ratiba).</p>
          )}
        </div>
      )}
    </div>
  );
}

function statusClass(s: LoanStatus) {
  switch (s) {
    case "PENDING":
      return "bg-muted text-foreground";
    case "UNDER_REVIEW":
      return "bg-primary/15 text-primary";
    case "APPROVED":
      return "bg-primary/15 text-primary";
    case "OUTSTANDING":
      return "bg-warning/25 text-foreground";
    case "CLOSED":
      return "bg-success/15 text-success";
    case "REJECTED":
      return "bg-destructive/10 text-destructive";
    default:
      return "bg-muted text-foreground";
  }
}

/** Client-side preview of amortized reducing-balance interest (display only). */
function equalInstallmentInterest(principal: number, monthlyRate: number, months: number): number {
  if (monthlyRate <= 0 || months <= 0) return 0;
  const pow = Math.pow(1 + monthlyRate, months);
  const payment = (principal * monthlyRate * pow) / (pow - 1);
  return Math.round((payment * months - principal) * 100) / 100;
}

function RequestForm({
  memberId,
  onClose,
}: {
  memberId: string;
  onClose: () => void;
}) {
  const applyLoan = useApplyLoan();
  const { data: loanSettingsData } = useQuery({
    queryKey: ["groups", "loan-settings-apply"],
    queryFn: async () => {
      const g = await groupsApi.current();
      return loanSettingsApi.get(g.data.id);
    },
    staleTime: 5 * 60 * 1000,
  });
  const ls = loanSettingsData?.data;
  const minDays = ls?.min_term_days ?? 30;
  const maxDays = ls?.max_term_days ?? 365;
  const interestOn = !!ls?.interest_enabled;
  const rate = Number(ls?.default_interest_rate ?? 0);

  const [f, setF] = useState({
    kiasi: "200000",
    muda: "90",
    maelezo: "",
  });

  const termDays = Math.max(0, parseInt(f.muda) || 0);
  const principal = Number(f.kiasi) || 0;
  // Client-side preview mirrors the backend flat/reducing math (monthly rate
  // over ceil(term/30) months). The server is authoritative; this is display only.
  const months = Math.max(1, Math.round(termDays / 30));
  const previewInterest = !interestOn || principal <= 0 || termDays <= 0
    ? 0
    : ls?.interest_type === "reducing"
      ? equalInstallmentInterest(principal, rate / 100, months)
      : Math.round(principal * (rate / 100) * months * 100) / 100;
  const previewTotal = principal + previewInterest;
  const termValid = termDays >= minDays && termDays <= maxDays;

  const handleSubmit = async () => {
    if (!termValid) return;
    try {
      await applyLoan.mutateAsync({
        member_id: memberId,
        amount: principal,
        term_days: termDays,
        purpose: f.maelezo || undefined,
      });
      onClose();
    } catch {
      /* handled by RQ */
    }
  };

  return (
    <Modal title="Omba Mkopo" onClose={onClose}>
      <Field
        label="Kiasi unachoomba (TZS)"
        value={f.kiasi}
        onChange={(v) => setF({ ...f, kiasi: v })}
        type="number"
      />      <div className="mt-3">
        <Field
          label={`Muda wa Mkopo (siku, ${minDays}–${maxDays})`}
          value={f.muda}
          onChange={(v) => setF({ ...f, muda: v })}
          type="number"
        />
        {!termValid && termDays > 0 && (
          <p className="mt-1 text-xs text-destructive">
            Muda lazima uwe kati ya siku {minDays} na {maxDays}.
          </p>
        )}
      </div>
      {/* Interest preview: read-only snapshot display, never editable */}
      <div className="mt-3 rounded-xl bg-muted/60 px-3 py-2.5 text-sm">
        {interestOn ? (
          <div className="space-y-1">
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Riba (kikundi, /mwezi)</span>
              <span className="font-semibold">{rate}% ({ls?.interest_type === "reducing" ? "salio linalopungua" : "flat"})</span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Riba inayokadiriwa</span>
              <span className="font-semibold">{tzs(previewInterest)}</span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Jumla ya kurejesha</span>
              <span className="font-semibold">{tzs(previewTotal)}</span>
            </div>
          </div>
        ) : (
          <p className="text-xs font-semibold text-success">Huu ni mkopo usio na riba — utarejesha {tzs(principal)} pekee.</p>
        )}
      </div>
      <div className="mt-3">
        <Field
          label="Madhumuni"
          value={f.maelezo}
          onChange={(v) => setF({ ...f, maelezo: v })}
        />
      </div>
      {applyLoan.error && (
        <p className="mt-2 text-center text-xs text-destructive">{applyLoan.error.message}</p>
      )}
      <button
        disabled={applyLoan.isPending || !termValid || principal <= 0}
        onClick={handleSubmit}
        className="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-xl bg-primary py-3 text-sm font-semibold text-primary-foreground disabled:opacity-50"
      >
        {applyLoan.isPending ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Send className="h-4 w-4" />
        )}{" "}
        Tuma Ombi
      </button>
      <p className="mt-2 text-center text-xs text-muted-foreground">
        Mwenyekiti ataidhinisha kisha Mweka Hazina atakutolea fedha.
      </p>
    </Modal>
  );
}

function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-foreground/40 sm:items-center"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-t-3xl bg-card p-5 sm:rounded-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h3 className="font-display text-lg font-semibold">{title}</h3>
          <button onClick={onClose} className="rounded-lg p-1.5 hover:bg-muted">
            <X className="h-4 w-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
