import { createFileRoute } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppShell } from "@/components/AppShell";
import { Field } from "@/components/Field";
import { tzs, tarehe } from "@/lib/format";
import { blockAdminFromPage, requireAuth } from "@/lib/role-guards";
import { useAuth } from "@/lib/auth-provider";
import { useAppModal } from "@/components/AppModal";
import { uploadApi } from "@/api/upload";
import { loansApi } from "@/api/loans";
import {
  useRepayments,
  useSubmitRepayment,
} from "@/hooks/use-repayments";
import { useLoans } from "@/hooks/use-loans";
import {
  Send,
  Loader2,
  X,
  Receipt,
  Clock,
  CheckCircle2,
  XCircle,
  ImageIcon,
} from "lucide-react";

export const Route = createFileRoute("/marejesho")({
  head: () => ({
    meta: [
      { title: "Marejesho — Fanya Malipo ya Mkopo" },
      { name: "description", content: "Wasilisha marejesho ya mkopo wako kwa uthibitisho wa Mweka Hazina." },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
    blockAdminFromPage();
  },
  component: MarejeshoPage,
});

function MarejeshoPage() {
  const { user } = useAuth();
  const [open, setOpen] = useState(false);
  if (!user) return null;

  const myMemberId = user.member_id || null;
  const { data: loansData, isLoading: loansLoading } = useLoans({ limit: 200 });
  const myLoans = (loansData?.data ?? []).filter(
    (l) => myMemberId && l.member_id === myMemberId && (l.status === "OUTSTANDING" || l.status === "APPROVED")
  );
  const activeLoan = myLoans.find((l) => l.status === "OUTSTANDING") ?? myLoans[0] ?? null;

  const { data: historyData, isLoading: historyLoading } = useRepayments({ limit: 200 });
  // Member view is strictly own-only: leadership roles (e.g. katibu) must
  // NOT see other members' repayments here (taarifa page is the only
  // group-wide view).
  const history = (historyData?.data ?? []).filter(
    (r) => !myMemberId || r.member_id === myMemberId
  );

  return (
    <AppShell
      title="Marejesho"
      subtitle="Fanya malipo ya mkopo wako — yanathibitishwa na Mweka Hazina"
      action={
        activeLoan?.status === "OUTSTANDING" ? (
          <button
            onClick={() => setOpen(true)}
            className="inline-flex items-center gap-1.5 rounded-xl bg-primary px-3.5 py-2 text-sm font-semibold text-primary-foreground"
          >
            <Send className="h-4 w-4" /> Fanya Marejesho
          </button>
        ) : null
      }
    >
      {!myMemberId ? (
        <div className="card-surface p-8 text-center text-sm text-muted-foreground">
          Hujasajiliwa kama mwanachama bado.
        </div>
      ) : loansLoading ? (
        <div className="mt-4 flex justify-center"><Loader2 className="h-6 w-6 animate-spin text-primary" /></div>
      ) : !activeLoan ? (
        <div className="card-surface p-8 text-center">
          <CheckCircle2 className="mx-auto h-12 w-12 text-success/60" />
          <p className="mt-3 font-semibold">Huna mkopo unaoendelea</p>
          <p className="mt-1 text-sm text-muted-foreground">Ukitoa mkopo na ukatolewa fedha, utaweza kufanya marejesho hapa.</p>
        </div>
      ) : (
        <ActiveLoanCard loan={activeLoan} onPay={() => setOpen(true)} />
      )}

      <h2 className="mt-6 mb-2 font-display text-sm font-semibold">Historia ya marejesho yangu</h2>
      {historyLoading ? (
        <div className="flex justify-center py-6"><Loader2 className="h-6 w-6 animate-spin text-primary" /></div>
      ) : history.length === 0 ? (
        <div className="card-surface p-8 text-center text-sm text-muted-foreground">
          Hakuna marejesho bado.
        </div>
      ) : (
        <div className="card-surface divide-y divide-border">
          {history.map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-4 py-3">
              <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-success/15 text-success">
                <Receipt className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{tzs(r.amount)}</p>
                <p className="text-xs text-muted-foreground">{tarehe(r.paid_at)}</p>
                {r.review_reason && r.status === "REJECTED" && (
                  <p className="mt-0.5 text-xs text-destructive">Sababu: {r.review_reason}</p>
                )}
              </div>
              <StatusChip status={r.status} />
            </div>
          ))}
        </div>
      )}

      {open && activeLoan && (
        <RepaymentForm
          loanId={activeLoan.id}
          balance={Number(activeLoan.balance_remaining ?? activeLoan.approved_amount ?? activeLoan.amount)}
          onClose={() => setOpen(false)}
        />
      )}
    </AppShell>
  );
}

function StatusChip({ status }: { status: string }) {
  if (status === "CONFIRMED")
    return <span className="chip bg-success/15 text-success text-[10px]">Imethibitishwa</span>;
  if (status === "REJECTED")
    return <span className="chip bg-destructive/10 text-destructive text-[10px]">Imekataliwa</span>;
  return (
    <span className="chip bg-amber-100 text-amber-700 text-[10px] inline-flex items-center gap-1">
      <Clock className="h-3 w-3" /> Inasubiri
    </span>
  );
}

function ActiveLoanCard({ loan, onPay }: { loan: { id: string; amount: number; approved_amount?: number; balance_remaining?: number; due_date: string; term_days?: number; interest_enabled?: boolean; total_repayment?: number }; onPay: () => void }) {
  const total = Number(loan.total_repayment ?? loan.approved_amount ?? loan.amount);
  const bal = Number(loan.balance_remaining ?? total);
  const { data: schedData } = useQuery({
    queryKey: ["loans", "schedule", loan.id],
    queryFn: () => loansApi.schedule(loan.id),
  });
  const nextDue = (schedData?.data ?? []).find((s) => s.status !== "PAID") ?? null;

  return (
    <div className="hero-surface px-5 py-5">
      <p className="text-xs text-primary-foreground/70">Salio la mkopo wako</p>
      <p className="mt-1 font-display text-3xl font-extrabold">{tzs(bal)}</p>
      <p className="mt-1 text-xs text-primary-foreground/70">
        Jumla: {tzs(total)}{loan.interest_enabled ? "" : " (Hakuna Riba)"} · Mwisho: {tarehe(loan.due_date)}
      </p>
      {nextDue && (
        <p className="mt-1 text-xs text-primary-foreground/70">
          Awamu inayofuata: {tzs(Number(nextDue.total_amount))} ifikapo {tarehe(nextDue.due_date)}
        </p>
      )}
      <button
        onClick={onPay}
        className="mt-4 inline-flex w-full items-center justify-center gap-2 rounded-xl bg-white px-5 py-2.5 text-sm font-bold text-primary shadow-lg transition-transform hover:scale-[1.01] sm:w-auto"
      >
        <Send className="h-4 w-4" /> Fanya Marejesho
      </button>
    </div>
  );
}

function RepaymentForm({ loanId, balance, onClose }: { loanId: string; balance: number; onClose: () => void }) {
  const submit = useSubmitRepayment();
  const { showModal } = useAppModal();
  const fileRef = useRef<HTMLInputElement>(null);
  const [f, setF] = useState({ kiasi: "", ujumbe: "" });
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [uploadedUrl, setUploadedUrl] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const kiasiN = Number(f.kiasi);
  const valid = !isNaN(kiasiN) && isFinite(kiasiN) && kiasiN > 0;
  const tooMuch = valid && kiasiN > balance;
  const hasProof = !!uploadedUrl || f.ujumbe.trim() !== "";

  const handleFile = async (file: File) => {
    if (!file.type.startsWith("image/")) {
      setUploadError("Chagua picha tu (JPG, PNG, GIF, WebP)");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      setUploadError("Picha ni kubwa mno. Kiwango cha juu ni 5MB");
      return;
    }
    setUploading(true);
    setUploadError(null);
    try {
      const res = await uploadApi.doc(file, "contributions");
      setUploadedUrl(res.url);
      const reader = new FileReader();
      reader.onload = (ev) => setPreviewUrl(ev.target?.result as string);
      reader.readAsDataURL(file);
    } catch (e: unknown) {
      setUploadError(e instanceof Error ? e.message : "Imeshindikana kupakia picha");
    } finally {
      setUploading(false);
    }
  };

  const handleSubmit = async () => {
    if (!valid || tooMuch || !hasProof) return;
    try {
      const res = await submit.mutateAsync({
        loanId,
        data: {
          amount: kiasiN,
          paid_at: new Date().toISOString().slice(0, 10),
          payment_method: "CASH",
          proof_image_url: uploadedUrl ?? undefined,
          proof_message: f.ujumbe.trim() || undefined,
        },
      });
      onClose();
      showModal({
        title: "Imewasilishwa",
        message: res.message || "Malipo yamewasilishwa. Inasubiri uthibitisho wa Mweka Hazina — salio halitapungua hadi athibitishe.",
        variant: "success",
        primaryLabel: "Sawa",
      });
    } catch {
      /* handled by mutation error display below */
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-foreground/40 sm:items-center" onClick={onClose}>
      <div className="max-h-[85dvh] w-full max-w-md overflow-y-auto rounded-t-3xl bg-card p-5 sm:rounded-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="mb-4 flex items-center justify-between">
          <h3 className="font-display text-lg font-semibold">Fanya Marejesho</h3>
          <button onClick={onClose} className="rounded-lg p-1.5 hover:bg-muted"><X className="h-4 w-4" /></button>
        </div>
        <div className="rounded-xl bg-muted p-3 text-xs">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Salio la mkopo</span>
            <span className="font-semibold">{tzs(balance)}</span>
          </div>
        </div>
        <div className="mt-3">
          <Field label="Kiasi cha malipo (TZS)" value={f.kiasi} onChange={(v) => setF({ ...f, kiasi: v })} type="number" />
        </div>
        {tooMuch && <p className="mt-1 text-xs text-destructive">Kiasi kinazidi salio ({tzs(balance)}).</p>}
        <div className="mt-3">
          <span className="mb-1 block text-xs font-medium text-muted-foreground">Uthibitisho (picha ya muamala)</span>
          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => { const fl = e.target.files?.[0]; if (fl) handleFile(fl); e.target.value = ""; }}
          />
          {previewUrl ? (
            <div className="relative">
              <img src={previewUrl} alt="uthibitisho" className="w-full rounded-xl border border-border object-cover" />
              <button
                onClick={() => { setPreviewUrl(null); setUploadedUrl(null); }}
                className="absolute right-2 top-2 rounded-lg bg-foreground/70 px-2 py-1 text-xs text-white"
              >
                Ondoa
              </button>
            </div>
          ) : (
            <button
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
              className="flex w-full items-center justify-center gap-2 rounded-xl border border-dashed border-border py-3 text-xs text-muted-foreground hover:bg-muted disabled:opacity-50"
            >
              {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <ImageIcon className="h-4 w-4" />}
              {uploading ? "Inapakia…" : "Chagua picha"}
            </button>
          )}
          {uploadError && <p className="mt-1 text-xs text-destructive">{uploadError}</p>}
        </div>
        <div className="mt-3">
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-muted-foreground">Ujumbe wa muamala (kama hauna picha)</span>
            <textarea
              value={f.ujumbe}
              onChange={(e) => setF({ ...f, ujumbe: e.target.value })}
              placeholder="Mf. TZS 50,000 Tigo Pesa, muamala MP…"
              rows={2}
              className="w-full rounded-xl border border-input bg-background px-3 py-2.5 text-sm outline-none focus:border-primary"
            />
          </label>
        </div>
        {!hasProof && <p className="mt-1 text-xs text-muted-foreground">Weka picha au ujumbe wa muamala (kimoja kinahitajika).</p>}
        {submit.error && (
          <p className="mt-2 rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">{submit.error.message}</p>
        )}
        <button
          disabled={!valid || tooMuch || !hasProof || uploading || submit.isPending}
          onClick={handleSubmit}
          className="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-xl bg-primary py-3 text-sm font-semibold text-primary-foreground disabled:opacity-50"
        >
          {submit.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          Wasilisha Marejesho
        </button>
        <p className="mt-2 text-center text-xs text-muted-foreground">
          Salio halitapungua hadi Mweka Hazina athibitishe.
        </p>
      </div>
    </div>
  );
}
