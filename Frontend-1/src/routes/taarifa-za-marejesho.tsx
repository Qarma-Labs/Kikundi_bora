import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import { useRepayments, usePendingRepayments, useApproveRepayment, useRejectRepayment } from "@/hooks/use-repayments";
import { useAuth } from "@/lib/auth-provider";
import { useAppModal } from "@/components/AppModal";
import { groupsApi } from "@/api/groups";
import { withUploadToken } from "@/api/upload";
import type { Repayment } from "@/api/types";
import { tzs, tarehe } from "@/lib/format";
import { blockAdminFromPage, requireAuth } from "@/lib/role-guards";
import { Skeleton } from "@/components/ui/skeleton";
import { X, Receipt, Loader2, MessageSquare, Check, Clock, Eye, ImageIcon } from "lucide-react";

export const Route = createFileRoute("/taarifa-za-marejesho")({
  beforeLoad: () => { requireAuth(); blockAdminFromPage(); },
  component: TaarifaZaMarejeshoPage,
});

function TaarifaZaMarejeshoPage() {
  const { user } = useAuth();
  const [detail, setDetail] = useState<Repayment | null>(null);
  const isHazina = user?.role === "treasurer";

  const { data: repaymentsData, isLoading, error, refetch } = useRepayments({ limit: 200 });
  const repayments = repaymentsData?.data ?? [];
  // Total counts CONFIRMED repayments only — pending submissions must never
  // inflate the received figure.
  const confirmed = repayments.filter((r) => r.status === "CONFIRMED");
  const pendingCount = repayments.filter((r) => r.status === "PENDING").length;
  const jumla = confirmed.reduce((s, r) => s + Number(r.amount), 0);

  return (
    <AppShell
      title="Taarifa Za Marejesho"
      subtitle="Angalia taarifa za marejesho ya wanachama"
    >
      <div className="card-surface p-5">
        <p className="text-xs text-muted-foreground">Jumla ya marejesho yaliyopokelewa (yaliyothibitishwa)</p>
        <p className="mt-1 font-display text-3xl font-extrabold text-success">{tzs(jumla)}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          {confirmed.length} yaliyothibitishwa{pendingCount > 0 ? ` · ${pendingCount} yanasubiri` : ""}
        </p>
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
                  <div className="flex items-center gap-2">
                    <p className="truncate text-sm font-medium">{r.member?.full_name ?? `Mwanachama #${r.member_id}`}</p>
                    {r.status === "PENDING" && (
                      <span className="chip bg-amber-100 text-amber-700 text-[10px] inline-flex items-center gap-1">
                        <Clock className="h-3 w-3" /> Inasubiri
                      </span>
                    )}
                    {r.status === "REJECTED" && (
                      <span className="chip bg-destructive/10 text-destructive text-[10px]">Imekataliwa</span>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">{tarehe(r.paid_at)} · Salio: {tzs(r.balance_after)}</p>
                  {r.notes && (
                    <p className="text-xs text-muted-foreground/70 mt-0.5 flex items-center gap-1">
                      <MessageSquare className="h-3 w-3" /> {r.notes}
                    </p>
                  )}
                </div>
                <button
                  onClick={() => setDetail(r)}
                  title="Tazama maelezo ya rejesho"
                  aria-label={`Maelezo ya rejesho ${tzs(r.amount)}`}
                  className="rounded-lg p-1.5 text-primary hover:bg-primary/10"
                >
                  <Eye className="h-4 w-4" />
                </button>
                <p className="shrink-0 text-sm font-semibold text-success">+{tzs(r.amount)}</p>
              </div>
            ))}
            {repayments.length === 0 && <p className="px-4 py-8 text-center text-sm text-muted-foreground">Hakuna marejesho bado.</p>}
          </div>
        </>
      )}

      {detail && <RepaymentDetailsModal repayment={detail} onClose={() => setDetail(null)} />}
    </AppShell>
  );
}

function RepaymentDetailsModal({ repayment: r, onClose }: { repayment: Repayment; onClose: () => void }) {
  const proofSrc = withUploadToken(r.proof_image_url);
  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-foreground/40 sm:items-center" onClick={onClose}>
      <div className="max-h-[85dvh] w-full max-w-md overflow-y-auto rounded-t-3xl bg-card p-5 sm:rounded-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="mb-4 flex items-center justify-between">
          <h3 className="font-display text-lg font-semibold">Maelezo ya Rejesho</h3>
          <button onClick={onClose} className="rounded-lg p-1.5 hover:bg-muted"><X className="h-4 w-4" /></button>
        </div>
        <div className="flex items-center gap-3">
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-success/15 text-success">
            <Receipt className="h-5 w-5" />
          </div>
          <div>
            <p className="font-display text-xl font-bold">{tzs(r.amount)}</p>
            <p className="text-xs text-muted-foreground">{r.member?.full_name ?? `Mwanachama #${r.member_id}`}</p>
          </div>
          <span className={`ml-auto chip text-[10px] ${r.status === "CONFIRMED" ? "bg-success/15 text-success" : r.status === "REJECTED" ? "bg-destructive/10 text-destructive" : "bg-amber-100 text-amber-700"}`}>
            {r.status === "CONFIRMED" ? "Imethibitishwa" : r.status === "REJECTED" ? "Imekataliwa" : "Inasubiri uthibitisho"}
          </span>
        </div>
        <dl className="mt-4 space-y-2 text-sm">
          <div className="flex justify-between rounded-lg bg-muted/50 px-3 py-2"><dt className="text-muted-foreground">Tarehe ya malipo</dt><dd className="font-medium">{tarehe(r.paid_at)}</dd></div>
          <div className="flex justify-between rounded-lg bg-muted/50 px-3 py-2"><dt className="text-muted-foreground">Njia</dt><dd className="font-medium">{r.payment_method}</dd></div>
          <div className="flex justify-between rounded-lg bg-muted/50 px-3 py-2"><dt className="text-muted-foreground">Salio baada</dt><dd className="font-medium">{tzs(r.balance_after)}</dd></div>
          {r.notes && <div className="rounded-lg bg-muted/50 px-3 py-2"><dt className="text-muted-foreground text-xs">Maelezo</dt><dd className="mt-0.5 font-medium">{r.notes}</dd></div>}
          {r.proof_message && <div className="rounded-lg bg-muted/50 px-3 py-2"><dt className="text-muted-foreground text-xs">Ujumbe wa muamala</dt><dd className="mt-0.5 font-medium">“{r.proof_message}”</dd></div>}
          {r.review_reason && <div className="rounded-lg bg-destructive/10 px-3 py-2"><dt className="text-xs text-destructive">Sababu ya kukataliwa</dt><dd className="mt-0.5 font-medium text-destructive">{r.review_reason}</dd></div>}
        </dl>
        {proofSrc && (
          <a href={proofSrc} target="_blank" rel="noreferrer" className="mt-3 block">
            <p className="mb-1 flex items-center gap-1 text-xs font-semibold text-muted-foreground"><ImageIcon className="h-3.5 w-3.5" /> Picha ya uthibitisho</p>
            <img src={proofSrc} alt="uthibitisho wa malipo" className="max-h-64 w-full rounded-xl border border-border object-contain" />
          </a>
        )}
        {r.status === "PENDING" && (
          <p className="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800">
            Rejesho hili bado halijaidhinishwa — halijapunguza salio. Mweka Hazina atalithibitisha kwenye “Thibitisha Marejesho”.
          </p>
        )}
      </div>
    </div>
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
