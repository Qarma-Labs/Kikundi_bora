import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import { Field } from "@/components/Field";
import { useAuth } from "@/lib/auth-provider";
import { requireAuth, requireRole, hasRole } from "@/lib/role-guards";
import {
  useCollectionQueue,
  useCollectFine,
  obligationKeys,
} from "@/hooks/use-obligations";
import {
  usePendingRepayments,
  useApproveRepayment,
  useRejectRepayment,
  repaymentKeys,
} from "@/hooks/use-repayments";
import { groupsApi } from "@/api/groups";
import { contributionsApi } from "@/api/contributions";
import { tzs } from "@/lib/format";
import { HandCoins, Loader2, Check, Banknote } from "lucide-react";
import { useAppModal } from "@/components/AppModal";

export const Route = createFileRoute("/ukusanyaji")({
  head: () => ({
    meta: [
      { title: "Ukusanyaji — Money Seeking" },
      { name: "description", content: "Foleni ya ukusanyaji: malimbikizo na faini zinazosubiri (mweka hazina)." },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
    requireRole("treasurer");
  },
  component: UkusanyajiPage,
});

function UkusanyajiPage() {
  const { user } = useAuth();
  const { showModal } = useAppModal();
  const qc = useQueryClient();
  const { data: gs } = useQuery({
    queryKey: ["groups", "current"],
    queryFn: () => groupsApi.current(),
    staleTime: 5 * 60 * 1000,
  });
  const groupId = gs?.data.id ?? null;
  const { data, isLoading } = useCollectionQueue(groupId);
  const collect = useCollectFine();
  const [payFor, setPayFor] = useState<string | null>(null);

  if (!hasRole(user, "treasurer", "admin")) {
    return (
      <AppShell title="Ukusanyaji" subtitle="Huna ruhusa">
        <p className="text-sm text-muted-foreground">Ukurasa huu ni kwa Mweka Hazina tu.</p>
      </AppShell>
    );
  }

  const onCollect = (fineId: string, label: string) => {
    showModal({
      title: "Mark as Collected?",
      message: `Thibitisha umepokea ${label}.`,
      variant: "warning",
      primaryLabel: "Nimepokea",
      onPrimary: () =>
        collect.mutate(fineId, {
          onSuccess: (res: any) => {
            showModal({ title: "Imefanikiwa", message: res.message, variant: "success", primaryLabel: "Sawa" });
            qc.invalidateQueries({ queryKey: ["obligations"] });
          },
          onError: (e: Error) =>
            showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
        }),
    });
  };

  return (
    <AppShell title="Ukusanyaji" subtitle="Malimbikizo na faini — kubwa kwanza">
      {isLoading ? (
        <div className="mt-8 flex justify-center"><Loader2 className="h-6 w-6 animate-spin text-primary" /></div>
      ) : (data?.data.length ?? 0) === 0 ? (
        <p className="text-sm text-muted-foreground">Hakuna madeni — kila mwanachama yuko sawa.</p>
      ) : (
        <div className="max-w-3xl space-y-3">
          {(data?.data ?? []).map((entry) => (
            <section key={entry.member.member_id} className="card-surface overflow-hidden" data-testid={`queue-${entry.member.member_no}`}>
              <header className="flex items-center justify-between border-b border-border px-4 py-3">
                <div>
                  <h3 className="font-display text-sm font-semibold">{entry.member.full_name}</h3>
                  <p className="text-xs text-muted-foreground">{entry.member.member_no}</p>
                </div>
                <span className="text-sm font-bold">{tzs(Number(entry.member.grand_total_owed))}</span>
              </header>
              <div>
                {entry.arrears.map((a) => (
                  <div key={a.cycle_label} className="flex items-center justify-between border-t border-border px-4 py-2.5 first:border-t-0">
                    <span className="text-sm">Malimbikizo {a.cycle_label} <span className="text-xs text-muted-foreground">({new Date(a.due_date).toLocaleDateString()})</span></span>
                    <span className="text-sm font-semibold">{tzs(Number(a.owed))}</span>
                  </div>
                ))}
                {entry.fines.map((f) => (
                  <div key={f.id} className="flex items-center justify-between gap-2 border-t border-border px-4 py-2.5">
                    <span className="text-sm">Faini: {f.offence_name} <span className="text-xs text-muted-foreground">({new Date(f.occurrence_date).toLocaleDateString()})</span></span>
                    <span className="flex items-center gap-2">
                      <span className="text-sm font-semibold">{tzs(Number(f.amount))}</span>
                      <button
                        onClick={() => onCollect(f.id, `${f.offence_name} — ${tzs(Number(f.amount))}`)}
                        disabled={collect.isPending}
                        aria-label={`Mark as Collected ${f.offence_name}`}
                        className="inline-flex items-center gap-1 rounded-lg bg-primary px-2.5 py-1.5 text-xs font-semibold text-primary-foreground disabled:opacity-50"
                      >
                        <Check className="h-3.5 w-3.5" /> Pokea
                      </button>
                    </span>
                  </div>
                ))}
                {payFor === entry.member.member_id ? (
                  <RecordPaymentForm
                    memberId={entry.member.member_id}
                    memberName={entry.member.full_name}
                    suggested={entry.member.grand_total_owed}
                    onDone={() => {
                      setPayFor(null);
                      qc.invalidateQueries({ queryKey: obligationKeys.queue(groupId ?? "") });
                      qc.invalidateQueries({ queryKey: ["obligations"] });
                    }}
                    onCancel={() => setPayFor(null)}
                  />
                ) : (
                  <div className="border-t border-border px-4 py-2.5">
                    <button
                      onClick={() => setPayFor(entry.member.member_id)}
                      className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-semibold hover:bg-muted"
                    >
                      <Banknote className="h-3.5 w-3.5" /> Rekodi Malipo
                    </button>
                  </div>
                )}
              </div>
            </section>
          ))}
        </div>
      )}
      {/* Adjacent section: member-submitted loan repayments awaiting approval.
          Kept separate from arrears/fines so the queues are never confused. */}
      <div className="mt-8 max-w-3xl">
        <PendingRepaymentsSection groupId={groupId} />
      </div>
    </AppShell>
  );
}

function PendingRepaymentsSection({ groupId }: { groupId: string | null }) {
  const { showModal } = useAppModal();
  const qc = useQueryClient();
  const { data, isLoading } = usePendingRepayments(groupId);
  const approve = useApproveRepayment();
  const reject = useRejectRepayment();
  const [rejectFor, setRejectFor] = useState<string | null>(null);
  const [reason, setReason] = useState("");

  const rows = data?.data ?? [];
  const refresh = () => qc.invalidateQueries({ queryKey: repaymentKeys.all });

  const doApprove = (id: string, label: string) =>
    showModal({
      title: "Thibitisha Marejesho?",
      message: `Unathibitisha umepokea ${label}. Salio la mkopo litapungua mara moja.`,
      variant: "warning",
      primaryLabel: "Nimethibitisha",
      secondaryLabel: "Ghairi",
      onPrimary: () =>
        approve.mutate(id, {
          onSuccess: (res) => {
            refresh();
            showModal({ title: "Imefanikiwa", message: res.message, variant: "success", primaryLabel: "Sawa" });
          },
          onError: (e: Error) =>
            showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
        }),
    });

  const doReject = (id: string) => {
    if (!reason.trim()) return;
    reject.mutate(
      { id, reason: reason.trim() },
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
    );
  };

  return (
    <section className="card-surface p-4" data-testid="pending-repayments">
      <h3 className="font-display text-sm font-semibold">Marejesho Yanayosubiri</h3>
      <p className="text-xs text-muted-foreground">Malipo yaliyowasilishwa na wanachama — thibitisha kupokea au kataa na sababu.</p>
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
                    {r.member?.member_no} · {tzs(Number(r.amount))} · {new Date(r.paid_at).toLocaleDateString()}
                  </p>
                  {r.proof_message && <p className="mt-1 text-xs text-muted-foreground">“{r.proof_message}”</p>}
                </div>
                <span className="chip bg-amber-100 text-amber-700 text-[10px]">Inasubiri</span>
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
                      onClick={() => doReject(r.id)}
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
                    onClick={() => doApprove(r.id, `${r.member?.full_name ?? ""} — ${tzs(Number(r.amount))}`)}
                    disabled={approve.isPending}
                    className="inline-flex items-center gap-1 rounded-lg bg-success px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
                  >
                    <Check className="h-3.5 w-3.5" /> Thibitisha
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
    </section>
  );
}

function RecordPaymentForm({ memberId, memberName, suggested, onDone, onCancel }: {
  memberId: string;
  memberName: string;
  suggested: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { showModal } = useAppModal();
  const [amount, setAmount] = useState(suggested);
  const [paidAt, setPaidAt] = useState(new Date().toISOString().slice(0, 10));
  const mutation = useMutation({
    mutationFn: () =>
      contributionsApi.create({
        member_id: memberId,
        amount: parseFloat(amount),
        month: new Date().toISOString().slice(0, 7),
        paid_at: paidAt,
        payment_method: "CASH",
      }),
    onSuccess: (res: any) => {
      const lines = (res.receipt?.lines ?? res.data?.lines ?? [])
        .map((l: any) => `${l.label}: ${tzs(Number(l.amount))}`)
        .join("\n");
      showModal({
        title: "Malipo yamegawanywa",
        message: `Malipo ya ${memberName} yamegawanywa:\n${lines}`,
        variant: "success",
        primaryLabel: "Sawa",
      });
      onDone();
    },
    onError: (e: Error) =>
      showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
  });

  return (
    <div className="space-y-2 border-t border-border bg-muted/20 px-4 py-3">
      <p className="text-xs font-semibold">Rekodi malipo — {memberName} (deni lote: {tzs(Number(suggested))})</p>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Kiasi (TZS)" type="number" value={amount} onChange={setAmount} placeholder="0" />
        <Field label="Tarehe" type="date" value={paidAt} onChange={setPaidAt} />
      </div>
      <div className="flex gap-2">
        <button
          onClick={() => mutation.mutate()}
          disabled={mutation.isPending || !(parseFloat(amount) > 0)}
          className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-2 text-xs font-semibold text-primary-foreground disabled:opacity-50"
        >
          {mutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <HandCoins className="h-3.5 w-3.5" />}
          Gawa &amp; Hifadhi
        </button>
        <button onClick={onCancel} className="rounded-lg border border-border px-3 py-2 text-xs">Ghairi</button>
      </div>
      <p className="text-[11px] text-muted-foreground">Kiasi kitagawanywa: malimbikizo ya zamani → mchango wa sasa → faini.</p>
    </div>
  );
}
