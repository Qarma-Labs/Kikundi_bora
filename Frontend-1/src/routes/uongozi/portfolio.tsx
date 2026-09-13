import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/lib/auth-provider";
import { requireAuth, requireRole } from "@/lib/role-guards";
import { AppShell } from "@/components/AppShell";
import { AppModal, useAppModal } from "@/components/AppModal";
import { loansApi, loanOffsetApi, type PortfolioLoan, type LoanOffset } from "@/api/loans";
import { api } from "@/api/client";
import { tzs, tarehe } from "@/lib/format";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Briefcase, Loader2, Banknote, AlertCircle, CheckCircle2, Wallet, X,
} from "lucide-react";

export const Route = createFileRoute("/uongozi/portfolio")({
  beforeLoad: () => {
    requireAuth();
    requireRole("chair", "secretary", "treasurer");
  },
  component: PortfolioPage,
});

interface Repayment {
  id: string;
  loan_id: string;
  amount: string;
  paid_at: string;
  payment_method: string;
  member?: { full_name?: string };
}

function PortfolioPage() {
  const { user } = useAuth();
  const { showModal } = useAppModal();

  // Filters
  const [status, setStatus] = useState("");
  const [memberId, setMemberId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const { data: portfolio, isLoading } = useQuery({
    queryKey: ["loan-portfolio", status, memberId, from, to],
    queryFn: () => loansApi.portfolio({ status, member_id: memberId, from, to }),
  });

  // Member filter options
  const { data: membersData } = useQuery({
    queryKey: ["members", "portfolio-filter"],
    queryFn: () =>
      api.get<{
        data: Array<{ id: string; full_name: string; member_no: string }>;
      }>("/members?limit=200"),
  });
  const members = (membersData?.data ?? []) as Array<{
    id: string;
    full_name: string;
    member_no: string;
  }>;

  const summary = portfolio?.data;
  const loans = summary?.loans ?? [];
  const [selected, setSelected] = useState<PortfolioLoan | null>(null);

  return (
    <AppShell
      title="Portfolio ya Mikopo"
      subtitle="Mikopo yote iliyotolewa — salio, malipo na milele ya marejesho"
    >
      {/* Summary cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <SummaryCard
          icon={Banknote}
          label="Jumla Iliyotolewa"
          value={tzs(Number(summary?.total_disbursed ?? 0))}
          color="bg-primary/10 text-primary"
        />
        <SummaryCard
          icon={Wallet}
          label="Salio la Mikopo"
          value={tzs(Number(summary?.total_outstanding ?? 0))}
          color="bg-amber-100 text-amber-700"
        />
        <SummaryCard
          icon={AlertCircle}
          label="Zilizochelewa"
          value={tzs(Number(summary?.total_overdue ?? 0))}
          sub={`${summary?.count_overdue ?? 0} mikopo`}
          color="bg-destructive/10 text-destructive"
        />
        <SummaryCard
          icon={CheckCircle2}
          label="Mikopo Wazi"
          value={String(summary?.count_outstanding ?? 0)}
          sub={`${summary?.count_closed ?? 0} zimefungwa`}
          color="bg-success/15 text-success"
        />
      </div>

      {/* Filters */}
      <div className="card-surface p-4 mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Hali</label>
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
          >
            <option value="">Zote</option>
            <option value="OUTSTANDING">Wazi (Inatumika)</option>
            <option value="CLOSED">Imefungwa (Imelipwa)</option>
          </select>
        </div>
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Mwanachama</label>
          <select
            value={memberId}
            onChange={(e) => setMemberId(e.target.value)}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
          >
            <option value="">Wote</option>
            {members.map((m) => (
              <option key={m.id} value={m.id}>
                {m.full_name} ({m.member_no})
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Kutoka (tarehe ya kutoa)</label>
          <input
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
          />
        </div>
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Hadi</label>
          <input
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
          />
        </div>
      </div>

      {/* Loans table */}
      {isLoading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
        </div>
      ) : loans.length === 0 ? (
        <div className="card-surface p-10 text-center mt-4">
          <Briefcase className="mx-auto h-10 w-10 text-muted-foreground/50" />
          <p className="mt-3 text-muted-foreground">Hakuna mikopo iliyotolewa kwenye kichuja hiki</p>
        </div>
      ) : (
        <div className="mt-4 space-y-2.5">
          {loans.map((l) => (
            <PortfolioRow key={l.id} loan={l} onClick={() => setSelected(l)} />
          ))}
        </div>
      )}

      {/* Loan detail + repayment history modal */}
      <AppModal
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
        title="Historia ya Mkopo"
        message={
          selected
            ? `${selected.full_name || "Mwanachama"} · ${tzs(Number(selected.principal))} · ${statusBadge(selected).label}`
            : ""
        }
        variant="info"
        primaryLabel="Funga"
      >
        {selected && (
          <div className="space-y-3 text-sm">
            <div className="grid grid-cols-2 gap-2 text-xs">
              <div>
                <p className="text-muted-foreground">Kiasi kilichotolewa</p>
                <p className="font-semibold">{tzs(Number(selected.principal))}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Tarehe ya kutoa</p>
                <p className="font-semibold">
                  {selected.disbursed_at ? tarehe(selected.disbursed_at) : "—"}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">Imelipwa</p>
                <p className="font-semibold text-success">{tzs(Number(selected.amount_repaid))}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Salio</p>
                <p className="font-semibold text-warning">{tzs(Number(selected.outstanding))}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Mwisho wa kulipa</p>
                <p className={`font-semibold ${selected.is_overdue ? "text-destructive" : ""}`}>
                  {tarehe(selected.due_date)}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">Hali</p>
                <p className="font-semibold">{statusBadge(selected).label}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Muda (siku)</p>
                <p className="font-semibold">{selected.term_days ?? "—"}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Riba</p>
                <p className="font-semibold">
                  {selected.interest_enabled
                    ? `${Number(selected.interest_rate)}% / mwezi · ${tzs(Number(selected.interest_amount))}`
                    : "Hakuna Riba"}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">Jumla ya kurejesha</p>
                <p className="font-semibold">{tzs(Number(selected.total_repayment ?? selected.principal))}</p>
              </div>
            </div>
            <div className="border-t pt-3">
              <p className="text-xs font-semibold text-muted-foreground mb-2">
                Marejesho yaliyorekodiwa
              </p>
              <RepaymentHistory loanId={selected.id} />
            </div>
            {selected.is_overdue && selected.status === "OUTSTANDING" && (
              <div className="border-t pt-3">
                <OffsetSection loan={selected} role={user?.role} />
              </div>
            )}
          </div>
        )}
      </AppModal>
    </AppShell>
  );
}

function RepaymentHistory({ loanId }: { loanId: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ["repayments", "loan", loanId],
    queryFn: () =>
      api.get<{ data: Repayment[] }>(`/repayments?loan_id=${loanId}&limit=100`),
  });
  const rows = data?.data ?? [];

  if (isLoading)
    return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />;
  if (rows.length === 0)
    return <p className="text-xs text-muted-foreground">Hakuna marejesho yaliyorekodiwa bado.</p>;

  return (
    <div className="rounded-lg border divide-y divide-border max-h-48 overflow-y-auto">
      {rows.map((r) => (
        <div key={r.id} className="flex items-center justify-between px-3 py-2 text-xs">
          <div>
            <p className="font-semibold">{tzs(Number(r.amount))}</p>
            <p className="text-muted-foreground">
              {new Date(r.paid_at).toLocaleDateString("sw-TZ")} · {r.payment_method}
            </p>
          </div>
          <CheckCircle2 className="h-4 w-4 text-success" />
        </div>
      ))}
    </div>
  );
}

/**
 * Offset with Contributions — overdue loans only. Three-role flow:
 * chair proposes → secretary approves/rejects → treasurer executes.
 * Preview shows outstanding, available savings and the capped amount
 * BEFORE anything irreversible happens (confirm via AppModal).
 */
function OffsetSection({ loan, role }: { loan: PortfolioLoan; role?: string }) {
  const qc = useQueryClient();
  const { showModal } = useAppModal();

  const previewQ = useQuery({
    queryKey: ["loan-offset-preview", loan.id],
    queryFn: () => loanOffsetApi.preview(loan.id),
  });
  const offsetsQ = useQuery({
    queryKey: ["loan-offsets", loan.id],
    queryFn: () => loanOffsetApi.list({ loan_id: loan.id }),
  });
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["loan-offset-preview", loan.id] });
    qc.invalidateQueries({ queryKey: ["loan-offsets", loan.id] });
    qc.invalidateQueries({ queryKey: ["loan-portfolio"] });
  };

  const mkMut = <T,>(fn: (v: T) => Promise<{ message: string }>, okTitle: string) => ({
    mutationFn: fn,
    onSuccess: (r: { message: string }) => {
      refresh();
      showModal({ title: okTitle, message: r.message, variant: "success", primaryLabel: "Sawa" });
    },
    onError: (e: Error) => showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
  });
  const propose = useMutation(mkMut((_v: void) => loanOffsetApi.propose(loan.id), "Imetumwa"));
  const approve = useMutation(mkMut((id: string) => loanOffsetApi.approve(id), "Imeidhinishwa"));
  const reject = useMutation(mkMut((id: string) => loanOffsetApi.reject(id), "Imekataliwa"));
  const execute = useMutation(mkMut((id: string) => loanOffsetApi.execute(id), "Imetekelezwa"));

  const confirm = (title: string, message: string, onPrimary: () => void) =>
    showModal({ title, message, variant: "warning", primaryLabel: "Thibitisha", onPrimary });

  const preview = previewQ.data?.data;
  const offsets: LoanOffset[] = offsetsQ.data?.data ?? [];
  const pending = offsets.find((o) => o.status === "PROPOSED" || o.status === "APPROVED");

  return (
    <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-xs font-semibold text-destructive mb-2">
        Offset na Akiba — mkopo umechelewa
      </p>
      {previewQ.isLoading ? (
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      ) : preview ? (
        <>
          <div className="grid grid-cols-3 gap-2 text-xs mb-2">
            <div>
              <p className="text-muted-foreground">Salio la mkopo{preview.interest_enabled ? " (pamoja na riba)" : ""}</p>
              <p className="font-semibold">{tzs(Number(preview.outstanding))}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Akiba inayopatikana</p>
              <p className="font-semibold">{tzs(Number(preview.available_savings))}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Kiasi kitakachokatwa</p>
              <p className="font-semibold text-destructive">{tzs(Number(preview.offset_amount))}</p>
            </div>
          </div>
          <p className="mb-2 text-[11px] text-muted-foreground">
            {preview.interest_enabled
              ? `Kuu: ${tzs(Number(preview.principal))} + Riba ${Number(preview.interest_rate)}%: salio linajumuisha riba.`
              : "Mkopo huu hauna riba — salio ni kiasi kikuu pekee."}
          </p>
        </>
      ) : null}
      {!preview?.eligible && preview?.reason && (
        <p className="text-xs text-muted-foreground mb-2">{preview.reason}</p>
      )}

      {pending ? (
        <div className="rounded-lg bg-card border px-3 py-2 text-xs mb-2">
          <p className="font-semibold">
            Pendekezo: {tzs(Number(pending.status === "EXECUTED" ? pending.amount : pending.proposed_amount))} ·{" "}
            {pending.status === "PROPOSED" ? "Linasubiri Katibu" : "Linasubiri Hazina kutekeleza"}
          </p>
          <div className="mt-2 flex gap-2">
            {pending.status === "PROPOSED" && role === "secretary" && (
              <>
                <button
                  onClick={() => confirm("Idhinisha offset?", `Kiasi ${tzs(Number(pending.proposed_amount))} kitatumika kutoka akiba ya mwanachama kulipa mkopo huu uliochelewa.`, () => approve.mutate(pending.id))}
                  disabled={approve.isPending}
                  className="rounded-lg bg-success px-3 py-1.5 font-semibold text-white disabled:opacity-50"
                >
                  Idhinisha
                </button>
                <button
                  onClick={() => confirm("Kataa offset?", "Pendekezo litafutwa.", () => reject.mutate(pending.id))}
                  disabled={reject.isPending}
                  className="rounded-lg border border-border px-3 py-1.5 font-semibold disabled:opacity-50"
                >
                  Kataa
                </button>
              </>
            )}
            {pending.status === "APPROVED" && role === "treasurer" && (
              <button
                onClick={() => confirm("Tekeleza offset?", `Kiasi ${tzs(Number(pending.proposed_amount))} kitakatwa kutoka akiba na kupunguza salio la mkopo. Hatua hii hairudishwi nyuma.`, () => execute.mutate(pending.id))}
                disabled={execute.isPending}
                className="rounded-lg bg-primary px-3 py-1.5 font-semibold text-primary-foreground disabled:opacity-50"
              >
                Tekeleza (Hazina)
              </button>
            )}
          </div>
        </div>
      ) : (
        preview?.eligible && role === "chair" && (
          <button
            onClick={() => confirm("Pendekeza offset?", `Salio la mkopo: ${tzs(Number(preview.outstanding))} · Akiba inayopatikana: ${tzs(Number(preview.available_savings))} · Kitakachokatwa: ${tzs(Number(preview.offset_amount))}. Pendekezo litatumwa kwa Katibu.`, () => propose.mutate())}
            disabled={propose.isPending}
            className="rounded-lg bg-destructive px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
          >
            Pendekeza Offset na Akiba
          </button>
        )
      )}

      {offsets.filter((o) => o.status === "EXECUTED").length > 0 && (
        <div className="mt-2 space-y-1">
          {offsets.filter((o) => o.status === "EXECUTED").map((o) => (
            <p key={o.id} className="text-[11px] text-muted-foreground">
              ✓ Offset imetekelezwa: {tzs(Number(o.amount))} · {o.executed_at ? tarehe(o.executed_at) : ""}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
  sub,
  color,
}: {
  icon: React.ElementType;
  label: string;
  value: string;
  sub?: string;
  color: string;
}) {
  return (
    <div className="card-surface p-4 flex items-center gap-3">
      <span className={`grid h-11 w-11 shrink-0 place-items-center rounded-xl ${color}`}>
        <Icon className="h-5 w-5" />
      </span>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="font-display text-lg font-bold truncate">{value}</p>
        {sub && <p className="text-[10px] text-muted-foreground">{sub}</p>}
      </div>
    </div>
  );
}

function statusBadge(loan: PortfolioLoan) {
  if (loan.status === "CLOSED")
    return { label: "Imelipwa", cls: "bg-success/15 text-success" };
  if (loan.is_overdue)
    return { label: "Imechelewa", cls: "bg-destructive/10 text-destructive" };
  return { label: "Inatumika", cls: "bg-blue-100 text-blue-700" };
}

function PortfolioRow({ loan, onClick }: { loan: PortfolioLoan; onClick: () => void }) {
  const badge = statusBadge(loan);
  const pct =
    Number(loan.principal) > 0
      ? Math.min(100, (Number(loan.amount_repaid) / Number(loan.principal)) * 100)
      : 0;

  return (
    <div
      data-testid={`portfolio-loan-${loan.id}`}
      onClick={onClick}
      className={`card-surface p-4 cursor-pointer hover:border-primary/40 transition-colors ${
        loan.is_overdue ? "border-l-4 border-l-destructive" : ""
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate font-semibold">{loan.full_name || `Mwanachama #${loan.member_id}`}</p>
          <p className="text-xs text-muted-foreground">{loan.member_no}</p>
        </div>
        <span className={`chip text-[10px] ${badge.cls}`}>{badge.label}</span>
      </div>
      <div className="mt-3 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
        <div>
          <p className="text-muted-foreground">Kiasi</p>
          <p className="font-semibold">{tzs(Number(loan.principal))}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Imelipwa</p>
          <p className="font-semibold text-success">{tzs(Number(loan.amount_repaid))}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Salio</p>
          <p className={`font-semibold ${loan.outstanding !== "0" ? "text-warning" : ""}`}>
            {tzs(Number(loan.outstanding))}
          </p>
        </div>
        <div>
          <p className="text-muted-foreground">Mwisho</p>
          <p className={`font-semibold ${loan.is_overdue ? "text-destructive" : ""}`}>
            {tarehe(loan.due_date)}
          </p>
        </div>
        <div>
          <p className="text-muted-foreground">Muda (siku)</p>
          <p className="font-semibold">{loan.term_days ?? "—"}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Riba</p>
          <p className="font-semibold">
            {loan.interest_enabled ? `${Number(loan.interest_rate)}% / mwezi` : "Hakuna Riba"}
          </p>
        </div>
        <div className="col-span-2">
          <p className="text-muted-foreground">Jumla ya kurejesha</p>
          <p className="font-semibold">{tzs(Number(loan.total_repayment ?? loan.principal))}</p>
        </div>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
        <div className="h-full bg-success transition-all" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-1 text-[10px] text-muted-foreground">
        Ilitolewa {loan.disbursed_at ? tarehe(loan.disbursed_at) : "—"} · {pct.toFixed(0)}% imelipwa
      </p>
    </div>
  );
}
