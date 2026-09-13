import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import { requireAuth, requireRole } from "@/lib/role-guards";
import { useAppModal } from "@/components/AppModal";
import { groupsApi } from "@/api/groups";
import { loansApi } from "@/api/loans";
import { withUploadToken } from "@/api/upload";
import { tzs, tarehe } from "@/lib/format";
import {
  usePendingRepayments,
  useApproveRepayment,
  useRejectRepayment,
  repaymentKeys,
} from "@/hooks/use-repayments";
import type { PendingRepayment } from "@/api/types";
import { Loader2, Check, Receipt, ImageIcon } from "lucide-react";

export const Route = createFileRoute("/thibitisha-marejesho")({
  head: () => ({
    meta: [
      { title: "Thibitisha Marejesho — Mweka Hazina" },
      { name: "description", content: "Marejesho yaliyotumwa na wanachama — ona maelezo kamili na thibitisha." },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
    requireRole("treasurer");
  },
  component: ThibitishaMarejeshoPage,
});

function ThibitishaMarejeshoPage() {
  const qc = useQueryClient();
  const { showModal } = useAppModal();
  const [rejectFor, setRejectFor] = useState<string | null>(null);
  const [reason, setReason] = useState("");

  const { data: gs } = useQuery({
    queryKey: ["groups", "current"],
    queryFn: () => groupsApi.current(),
    staleTime: 5 * 60 * 1000,
  });
  const { data, isLoading } = usePendingRepayments(gs?.data.id ?? null);
  const approve = useApproveRepayment();
  const reject = useRejectRepayment();
  const rows = data?.data ?? [];

  const refresh = () => qc.invalidateQueries({ queryKey: repaymentKeys.all });

  return (
    <AppShell
      title="Thibitisha Marejesho"
      subtitle={`Malipo ${rows.length} yanayosubiri uthibitisho wako`}
    >
      {isLoading ? (
        <div className="flex justify-center py-12"><Loader2 className="h-8 w-8 animate-spin text-primary" /></div>
      ) : rows.length === 0 ? (
        <div className="card-surface p-12 text-center">
          <Receipt className="mx-auto h-12 w-12 text-muted-foreground/50" />
          <p className="mt-4 text-muted-foreground">Hakuna marejesho yanayosubiri hivi sasa.</p>
        </div>
      ) : (
        <div className="space-y-4">
          {rows.map((r) => (
            <PendingCard
              key={r.id}
              row={r}
              approving={approve.isPending}
              onApprove={() =>
                showModal({
                  title: "Thibitisha Umepokea?",
                  message: `Unathibitisha umepokea ${tzs(Number(r.amount))} kutoka ${r.member?.full_name ?? "mwanachama"}. Salio la mkopo litapungua mara moja.`,
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
              rejecting={rejectFor === r.id}
              reason={reason}
              onReasonChange={setReason}
              onAskReject={() => { setRejectFor(r.id); setReason(""); }}
              onCancelReject={() => { setRejectFor(null); setReason(""); }}
              onConfirmReject={() => {
                if (!reason.trim()) return;
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
                );
              }}
              rejectPending={reject.isPending}
            />
          ))}
        </div>
      )}
    </AppShell>
  );
}

function PendingCard({
  row: r,
  approving,
  onApprove,
  rejecting,
  reason,
  onReasonChange,
  onAskReject,
  onCancelReject,
  onConfirmReject,
  rejectPending,
}: {
  row: PendingRepayment;
  approving: boolean;
  onApprove: () => void;
  rejecting: boolean;
  reason: string;
  onReasonChange: (v: string) => void;
  onAskReject: () => void;
  onCancelReject: () => void;
  onConfirmReject: () => void;
  rejectPending: boolean;
}) {
  const { data: schedData } = useQuery({
    queryKey: ["loans", "schedule", r.loan_id],
    queryFn: () => loansApi.schedule(r.loan_id),
  });
  const nextDue = (schedData?.data ?? []).find((s) => s.status !== "PAID") ?? null;
  const proofSrc = withUploadToken(r.proof_image_url);

  return (
    <div className="card-surface p-5" data-testid={`pending-repayment-${r.id}`}>
      {/* Mwanachama */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate font-display text-lg font-semibold">{r.member?.full_name ?? "Mwanachama"}</p>
          <p className="text-xs text-muted-foreground">
            {r.member?.member_no} · {r.member?.phone} · {tarehe(r.paid_at)}
          </p>
        </div>
        <span className="chip bg-amber-100 text-amber-700 text-[10px]">Inasubiri</span>
      </div>

      {/* Mkopo */}
      <div className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
        <div className="rounded-lg bg-muted/50 px-3 py-2">
          <p className="text-muted-foreground">Kiasi kilichotumwa</p>
          <p className="font-semibold text-primary">{tzs(Number(r.amount))}</p>
        </div>
        <div className="rounded-lg bg-muted/50 px-3 py-2">
          <p className="text-muted-foreground">Salio la mkopo</p>
          <p className="font-semibold">{tzs(Number(r.loan?.balance_remaining ?? 0))}</p>
        </div>
        <div className="rounded-lg bg-muted/50 px-3 py-2">
          <p className="text-muted-foreground">Jumla ya mkopo</p>
          <p className="font-semibold">{tzs(Number(r.loan?.approved_amount ?? r.loan?.amount ?? 0))}</p>
        </div>
        <div className="rounded-lg bg-muted/50 px-3 py-2">
          <p className="text-muted-foreground">Awamu inayofuata</p>
          <p className="font-semibold">
            {nextDue ? `${tzs(Number(nextDue.total_amount))} · ${tarehe(nextDue.due_date)}` : "—"}
          </p>
        </div>
      </div>

      {/* Uthibitisho */}
      {(r.proof_message || proofSrc) && (
        <div className="mt-3 rounded-xl border border-border p-3">
          <p className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
            <ImageIcon className="h-3.5 w-3.5" /> Uthibitisho wa mwanachama
          </p>
          {r.proof_message && <p className="text-sm">“{r.proof_message}”</p>}
          {proofSrc && (
            <a href={proofSrc} target="_blank" rel="noreferrer" className="mt-2 block">
              <img src={proofSrc} alt="uthibitisho wa malipo" className="max-h-64 w-full rounded-lg border border-border object-contain" />
            </a>
          )}
        </div>
      )}

      {/* Actions — hazina pekee (route guard + backend 403) */}
      {rejecting ? (
        <div className="mt-3 space-y-2">
          <input
            value={reason}
            onChange={(e) => onReasonChange(e.target.value)}
            placeholder="Sababu ya kukataa (lazima)…"
            className="w-full rounded-xl border border-input bg-background px-3 py-2.5 text-sm"
          />
          <div className="flex gap-2">
            <button
              onClick={onConfirmReject}
              disabled={!reason.trim() || rejectPending}
              className="flex-1 rounded-xl bg-destructive py-2.5 text-sm font-semibold text-white disabled:opacity-50"
            >
              Thibitisha kukataa
            </button>
            <button onClick={onCancelReject} className="flex-1 rounded-xl border border-border py-2.5 text-sm font-semibold">
              Ghairi
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-3 flex gap-2">
          <button
            onClick={onApprove}
            disabled={approving}
            className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-xl bg-success py-2.5 text-sm font-semibold text-white disabled:opacity-50"
          >
            <Check className="h-4 w-4" /> Thibitisha Nimepokea
          </button>
          <button
            onClick={onAskReject}
            className="flex-1 rounded-xl border border-destructive/30 py-2.5 text-sm font-semibold text-destructive"
          >
            Kataa
          </button>
        </div>
      )}
    </div>
  );
}
