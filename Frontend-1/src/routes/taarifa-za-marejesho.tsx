import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import { useRepayments, useRecordRepayment, usePendingRepayments, useApproveRepayment, useRejectRepayment } from "@/hooks/use-repayments";
import { useLoans } from "@/hooks/use-loans";
import { useAuth } from "@/lib/auth-provider";
import { useAppModal } from "@/components/AppModal";
import { groupsApi } from "@/api/groups";
import { Field } from "@/components/Field";
import { tzs, tarehe } from "@/lib/format";
import { blockAdminFromPage, requireAuth } from "@/lib/role-guards";
import { Skeleton } from "@/components/ui/skeleton";
import { Plus, X, Receipt, Loader2, Wallet, MessageSquare, Check, Clock } from "lucide-react";

export const Route = createFileRoute("/taarifa-za-marejesho")({
  beforeLoad: () => { requireAuth(); blockAdminFromPage(); },
  component: TaarifaZaMarejeshoPage,
});

function TaarifaZaMarejeshoPage() {
  const { user } = useAuth();
  const [open, setOpen] = useState(false);
  const isHazina = user?.role === "treasurer";

  const { data: repaymentsData, isLoading, error, refetch } = useRepayments({ limit: 200 });
  const repayments = repaymentsData?.data ?? [];
  const jumla = repayments.reduce((s, r) => s + Number(r.amount), 0);

  return (
    <AppShell
      title="Taarifa Za Marejesho"
      subtitle={isHazina ? "Pokea na thibitisha malipo ya mikopo" : "Angalia taarifa za marejesho ya wanachama"}
      action={
        isHazina && (
          <button onClick={() => setOpen(true)} className="inline-flex items-center gap-1.5 rounded-xl bg-accent px-3.5 py-2 text-sm font-semibold text-accent-foreground">
            <Plus className="h-4 w-4" /> Pokea
          </button>
        )
      }
    >
      <div className="card-surface p-5">
        <p className="text-xs text-muted-foreground">Jumla ya marejesho yaliyopokelewa</p>
        <p className="mt-1 font-display text-3xl font-extrabold text-success">{tzs(jumla)}</p>
        <p className="mt-1 text-xs text-muted-foreground">{repayments.length} malipo</p>
      </div>

      {isHazina && <PendingReceiptSection />}

      {isLoading && (
        <div className="mt-4 space-y-2.5">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="card-surface flex items-center gap-3 p-3.5">
              <Skeleton className="h-9 w-9 shrink-0 rounded-xl" />
              <div className="flex-1 space-y-1.5"><Skeleton className="h-4 w-40" /><Skeleton className="h-3 w-28" /></div>
              <Skeleton className="h-4 w-20" />
            </div>
          ))}
        </div>
      )}

      {error && !isLoading && repayments.length === 0 && (
        <div className="card-surface mt-4 p-6 text-center">
          <p className="text-sm text-destructive mb-3">{error.message}</p>
          <button onClick={() => refetch()} className="inline-flex items-center gap-1.5 rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground">
            <Loader2 className="h-4 w-4" /> Jaribu tena
          </button>
        </div>
      )}

      {!isLoading && (
        <>
          <h2 className="mt-6 mb-2 font-display text-sm font-semibold">Historia ya marejesho</h2>
          <div className="card-surface divide-y divide-border">
            {repayments.map((r) => (
              <div key={r.id} className="flex items-center gap-3 px-4 py-3">
                <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-success/15 text-success">
                  <Receipt className="h-4 w-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{r.member?.full_name ?? `Mwanachama #${r.member_id}`}</p>
                  <p className="text-xs text-muted-foreground">{tarehe(r.paid_at)} · Salio: {tzs(r.balance_after)}</p>
                  {r.notes && (
                    <p className="text-xs text-muted-foreground/70 mt-0.5 flex items-center gap-1">
                      <MessageSquare className="h-3 w-3" /> {r.notes}
                    </p>
                  )}
                </div>
                <p className="shrink-0 text-sm font-semibold text-success">+{tzs(r.amount)}</p>
              </div>
            ))}
            {repayments.length === 0 && <p className="px-4 py-8 text-center text-sm text-muted-foreground">Hakuna marejesho bado.</p>}
          </div>
        </>
      )}

      {open && isHazina && <Form onClose={() => setOpen(false)} />}
    </AppShell>
  );
}

function PendingReceiptSection() {
  const { showModal } = useAppModal();
  const [rejectFor, setRejectFor] = useState<string | null>(null);
  const [reason, setReason] = useState("");

  const { data: gs } = useQuery({
    queryKey: ["groups", "current"],
    queryFn: () => groupsApi.current(),
    staleTime: 5 * 60 * 1000,
  });
  const { data, isLoading, refetch } = usePendingRepayments(gs?.data.id ?? null);
  const approve = useApproveRepayment();
  const reject = useRejectRepayment();
  const rows = data?.data ?? [];

  const refresh = () => {
    refetch();
  };

  return (
    <div className="card-surface mt-4 p-5" data-testid="pending-receipts">
      <h2 className="font-display text-sm font-semibold">Pokea Marejesho Yaliyotumwa ({rows.length})</h2>
      <p className="text-xs text-muted-foreground">Malipo ya wanachama yanayosubiri uthibitisho wako — salio litapungua ukithibitisha.</p>
      {isLoading ? (
        <div className="flex justify-center py-4"><Loader2 className="h-5 w-5 animate-spin text-primary" /></div>
      ) : rows.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">Hakuna marejesho yanayosubiri.</p>
      ) : (
        <div className="mt-3 space-y-2.5">
          {rows.map((r) => (
            <div key={r.id} className="rounded-xl border border-border p-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold">{r.member?.full_name ?? "Mwanachama"}</p>
                  <p className="text-xs text-muted-foreground">
                    {r.member?.member_no} · {tzs(Number(r.amount))} · {tarehe(r.paid_at)}
                  </p>
                  {r.proof_message && <p className="mt-1 text-xs text-muted-foreground">“{r.proof_message}”</p>}
                </div>
                <span className="chip bg-amber-100 text-amber-700 text-[10px] inline-flex items-center gap-1">
                  <Clock className="h-3 w-3" /> Inasubiri
                </span>
              </div>
              {rejectFor === r.id ? (
                <div className="mt-2 space-y-2">
                  <input
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="Sababu ya kukataa (lazima)…"
                    className="w-full rounded-lg border border-input bg-background px-3 py-2 text-xs"
                  />
                  <div className="flex gap-2">
                    <button
                      onClick={() =>
                        reject.mutate(
                          { id: r.id, reason: reason.trim() },
                          {
                            onSuccess: (res) => {
                              refresh();
                              setRejectFor(null);
                              setReason("");
                              showModal({ title: "Imekataliwa", message: res.message, variant: "success", primaryLabel: "Sawa" });
                            },
                            onError: (e: Error) =>
                              showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
                          }
                        )
                      }
                      disabled={!reason.trim() || reject.isPending}
                      className="rounded-lg bg-destructive px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
                    >
                      Thibitisha kukataa
                    </button>
                    <button onClick={() => { setRejectFor(null); setReason(""); }} className="rounded-lg border px-3 py-1.5 text-xs">
                      Ghairi
                    </button>
                  </div>
                </div>
              ) : (
                <div className="mt-2 flex gap-2">
                  <button
                    onClick={() =>
                      showModal({
                        title: "Pokea Marejesho?",
                        message: `Unathibitisha umepokea ${tzs(Number(r.amount))} kutoka ${r.member?.full_name ?? "mwanachama"}. Salio litapungua mara moja.`,
                        variant: "warning",
                        primaryLabel: "Nimepokea",
                        secondaryLabel: "Ghairi",
                        onPrimary: () =>
                          approve.mutate(r.id, {
                            onSuccess: (res) => {
                              refresh();
                              showModal({ title: "Imefanikiwa", message: res.message, variant: "success", primaryLabel: "Sawa" });
                            },
                            onError: (e: Error) =>
                              showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
                          }),
                      })
                    }
                    disabled={approve.isPending}
                    className="inline-flex items-center gap-1 rounded-lg bg-success px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
                  >
                    <Check className="h-3.5 w-3.5" /> Pokea
                  </button>
                  <button
                    onClick={() => { setRejectFor(r.id); setReason(""); }}
                    className="rounded-lg border border-destructive/30 px-3 py-1.5 text-xs font-semibold text-destructive"
                  >
                    Kataa
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Form({ onClose }: { onClose: () => void }) {
  const recordRepayment = useRecordRepayment();
  const { data: loansData } = useLoans({ status: "OUTSTANDING", limit: 100 });
  const openLoans = loansData?.data ?? [];

  const [loanId, setLoanId] = useState("");
  useEffect(() => { if (!loanId && openLoans[0]?.id) setLoanId(String(openLoans[0].id)); }, [openLoans]);
  const selectedLoan = openLoans.find((l) => String(l.id) === loanId);
  const bal = Number(selectedLoan?.balance_remaining ?? 0);

  const [f, setF] = useState({ kiasi: "", tarehe: new Date().toISOString().slice(0, 10), maelezo: "" });
  const kiasiN = Number(f.kiasi);
  const valid = !isNaN(kiasiN) && isFinite(kiasiN) && kiasiN > 0;
  const tooMuch = valid && kiasiN > bal;
  const [submitErr, setSubmitErr] = useState<string | null>(null);

  const resetForm = () => { setF({ kiasi: "", tarehe: new Date().toISOString().slice(0, 10), maelezo: "" }); setSubmitErr(null); onClose(); };

  const handleSubmit = async () => {
    if (!loanId || !valid) return;
    setSubmitErr(null);
    try {
      await recordRepayment.mutateAsync({
        loan_id: loanId, amount: kiasiN, paid_at: f.tarehe, payment_method: "CASH", notes: f.maelezo || undefined,
      });
      resetForm();
    } catch (e: unknown) {
      setSubmitErr(e instanceof Error ? e.message : "Imeshindikana kurekodi malipo");
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-foreground/40 sm:items-center" onClick={resetForm}>
      <div className="w-full max-w-md rounded-t-3xl bg-card p-5 sm:rounded-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="mb-4 flex items-center justify-between">
          <h3 className="font-display text-lg font-semibold">Pokea Marejesho</h3>
          <button onClick={resetForm} className="rounded-lg p-1.5 hover:bg-muted"><X className="h-4 w-4" /></button>
        </div>
        {submitErr && <p className="mb-3 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">{submitErr}</p>}
        {openLoans.length === 0 ? (
          <div className="rounded-xl bg-muted p-4 text-center text-sm text-muted-foreground space-y-2">
            <Wallet className="mx-auto h-8 w-8 text-muted-foreground/50" />
            <p>Hakuna mikopo iliyo wazi kwa sasa.</p>
          </div>
        ) : (
          <>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-muted-foreground">Mkopo</span>
              <select value={loanId} onChange={(e) => { setLoanId(e.target.value); setSubmitErr(null); }} className="w-full rounded-xl border border-input bg-background px-3 py-2.5 text-sm">
                {openLoans.map((l) => (
                  <option key={l.id} value={l.id}>{l.member?.full_name ?? `Mkopo #${l.id}`} · Salio: {tzs(l.balance_remaining ?? 0)}</option>
                ))}
              </select>
            </label>
            <div className="mt-3 grid grid-cols-2 gap-3 rounded-xl bg-muted p-3 text-xs">
              <div><p className="text-muted-foreground">Salio la mkopo</p><p className="font-semibold">{tzs(bal)}</p></div>
              <div><p className="text-muted-foreground">Mwisho</p><p className="font-semibold">{selectedLoan ? tarehe(selectedLoan.due_date) : "—"}</p></div>
            </div>
            <div className="mt-3"><Field label="Kiasi cha malipo (TZS)" value={f.kiasi} onChange={(v) => { setF({ ...f, kiasi: v }); setSubmitErr(null); }} type="number" /></div>
            <div className="mt-3"><Field label="Tarehe ya malipo" value={f.tarehe} onChange={(v) => setF({ ...f, tarehe: v })} type="date" /></div>
            <div className="mt-3">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-muted-foreground">Maelezo / Comment</span>
                <textarea
                  value={f.maelezo}
                  onChange={(e) => setF({ ...f, maelezo: e.target.value })}
                  placeholder="Andika maelezo ya malipo haya..."
                  rows={2}
                  className="w-full rounded-xl border border-input bg-background px-3 py-2.5 text-sm outline-none focus:border-primary"
                />
              </label>
            </div>
            {tooMuch && <p className="mt-2 text-xs text-destructive">Kiasi kinazidi salio la mkopo ({tzs(bal)}).</p>}
            <button
              disabled={!loanId || !valid || tooMuch || recordRepayment.isPending}
              onClick={handleSubmit}
              className="mt-5 w-full rounded-xl bg-primary py-3 text-sm font-semibold text-primary-foreground disabled:opacity-50 inline-flex items-center justify-center gap-2"
            >
              {recordRepayment.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Hifadhi Malipo
            </button>
          </>
        )}
      </div>
    </div>
  );
}
