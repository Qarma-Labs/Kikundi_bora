import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Percent, CalendarRange, CheckCircle2, Loader2, Landmark } from "lucide-react";
import { useAuth } from "@/lib/auth-provider";
import { useAppModal } from "@/components/AppModal";
import { groupsApi, loanSettingsApi } from "@/api/groups";

/**
 * Group loan settings card (riba + muda) for /mipangilio:
 *  - leadership sees the approved settings (interest on/off, rate, term window)
 *  - Mwenyekiti (chair) gets the propose form (blocked while ANY proposal is pending)
 *  - Katibu (secretary) gets Approve/Reject on the pending loan proposal
 *
 * Interest-free (interest_enabled=false) is a first-class mode: the form and
 * display never imply a rate exists for such groups.
 */
export function LoanSettingsCard() {
  const { user } = useAuth();
  const qc = useQueryClient();
  const { showModal } = useAppModal();
  const isChair = user?.role === "chair";
  const isSecretary = user?.role === "secretary";

  const { data: groupData } = useQuery({
    queryKey: ["groups", "settings"],
    queryFn: () => groupsApi.current(),
  });
  const groupId = groupData?.data?.id;

  const { data, isLoading } = useQuery({
    queryKey: ["groups", "loan-settings", groupId],
    queryFn: () => loanSettingsApi.get(groupId!),
    enabled: !!groupId,
  });

  const settings = data?.data;
  const pending = data?.pending_proposal ?? null;

  const [form, setForm] = useState({
    enabled: false,
    rate: "5",
    itype: "flat" as "flat" | "reducing",
    minDays: "14",
    maxDays: "365",
  });
  const [rejectMode, setRejectMode] = useState(false);
  const [rejectReason, setRejectReason] = useState("");

  useEffect(() => {
    if (settings) {
      setForm({
        enabled: settings.interest_enabled,
        rate: String(Number(settings.default_interest_rate) || 5),
        itype: settings.interest_type === "reducing" ? "reducing" : "flat",
        minDays: String(settings.min_term_days),
        maxDays: String(settings.max_term_days),
      });
    }
  }, [settings?.id]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["groups", "loan-settings"] });
  };

  const proposeMutation = useMutation({
    mutationFn: () =>
      loanSettingsApi.propose(groupId!, {
        interest_enabled: form.enabled,
        default_interest_rate: form.enabled ? parseFloat(form.rate) || 0 : undefined,
        interest_type: form.itype,
        min_term_days: parseInt(form.minDays) || 30,
        max_term_days: parseInt(form.maxDays) || 365,
      }),
    onSuccess: () => {
      showModal({ title: "Imefanikiwa", message: "Pendekezo limetumwa kwa Katibu kwa idhini.", variant: "success", primaryLabel: "Sawa" });
      invalidate();
    },
    onError: (e: Error) => showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
  });

  const approveMutation = useMutation({
    mutationFn: () => loanSettingsApi.approve(groupId!),
    onSuccess: () => {
      showModal({ title: "Imefanikiwa", message: "Mipangilio ya mikopo imeidhinishwa na sasa inatumika.", variant: "success", primaryLabel: "Sawa" });
      invalidate();
    },
    onError: (e: Error) => showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
  });

  const rejectMutation = useMutation({
    mutationFn: () => loanSettingsApi.reject(groupId!, rejectReason),
    onSuccess: () => {
      showModal({ title: "Imefanikiwa", message: "Pendekezo limekataliwa.", variant: "success", primaryLabel: "Sawa" });
      setRejectMode(false);
      setRejectReason("");
      invalidate();
    },
    onError: (e: Error) => showModal({ title: "Hitilafu", message: e.message, variant: "error", primaryLabel: "Sawa" }),
  });

  if (isLoading) {
    return (
      <div className="card-surface p-5">
        <Loader2 className="h-5 w-5 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="card-surface p-5">
      <h3 className="font-display text-base font-semibold flex items-center gap-2">
        <Landmark className="h-4 w-4 text-primary" /> Mipangilio ya Mikopo (Riba na Muda)
      </h3>

      {/* Current approved settings */}
      {settings && (
        <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
          <div className="rounded-lg bg-muted/50 px-3 py-2">
            <p className="text-xs text-muted-foreground">Riba</p>
            <p className="font-semibold">
              {settings.interest_enabled
                ? `${Number(settings.default_interest_rate)}% / mwezi (${settings.interest_type === "reducing" ? "salio linalopungua" : "flat"})`
                : "Hakuna Riba (interest-free)"}
            </p>
          </div>
          <div className="rounded-lg bg-muted/50 px-3 py-2">
            <p className="text-xs text-muted-foreground">Muda unaoruhusiwa</p>
            <p className="font-semibold">Siku {settings.min_term_days} – {settings.max_term_days}</p>
          </div>
        </div>
      )}

      {/* Pending proposal */}
      {pending && (
        <div className="mt-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm">
          <p className="font-semibold text-amber-900">Pendekezo linasubiri Katibu</p>
          <p className="mt-1 text-xs text-amber-800">
            Riba: {pending.loan_interest_enabled ? `${Number(pending.loan_default_interest_rate)}% / mwezi (${pending.loan_interest_type})` : "Hakuna"} ·{" "}
            Muda: siku {pending.loan_min_term_days} – {pending.loan_max_term_days}
          </p>
          {isSecretary && !rejectMode && (
            <div className="mt-2 flex gap-2">
              <button
                onClick={() => approveMutation.mutate()}
                disabled={approveMutation.isPending}
                className="inline-flex items-center gap-1 rounded-lg bg-success px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
              >
                <CheckCircle2 className="h-3.5 w-3.5" /> Idhinisha
              </button>
              <button
                onClick={() => setRejectMode(true)}
                className="rounded-lg border border-amber-300 px-3 py-1.5 text-xs font-semibold text-amber-900"
              >
                Kataa
              </button>
            </div>
          )}
          {isSecretary && rejectMode && (
            <div className="mt-2 space-y-2">
              <input
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                placeholder="Sababu ya kukataa…"
                className="w-full rounded-lg border border-amber-300 bg-white px-3 py-2 text-xs"
              />
              <div className="flex gap-2">
                <button
                  onClick={() => rejectMutation.mutate()}
                  disabled={!rejectReason || rejectMutation.isPending}
                  className="rounded-lg bg-destructive px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-50"
                >
                  Thibitisha kukataa
                </button>
                <button onClick={() => setRejectMode(false)} className="rounded-lg border px-3 py-1.5 text-xs">
                  Ghairi
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Chair propose form */}
      {isChair && !pending && (
        <div className="mt-4 space-y-3 border-t border-border pt-4">
          <label className="flex cursor-pointer items-center justify-between gap-2">
            <span className="text-sm font-medium inline-flex items-center gap-1.5">
              <Percent className="h-4 w-4 text-muted-foreground" /> Wezesha riba ya mikopo?
            </span>
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              className="h-5 w-9 appearance-none rounded-full bg-muted transition-colors checked:bg-primary relative cursor-pointer before:absolute before:top-0.5 before:left-0.5 before:h-4 before:w-4 before:rounded-full before:bg-white before:transition-transform checked:before:translate-x-4"
            />
          </label>
          {!form.enabled && (
            <p className="rounded-lg bg-success/10 px-3 py-2 text-xs text-success">
              Mikopo itakuwa interest-free — hakuna riba itakayotozwa wala kuonyeshwa popote.
            </p>
          )}
          {form.enabled && (
            <div className="grid grid-cols-2 gap-2">
              <label className="block">
                <span className="mb-1 block text-xs text-muted-foreground">Kiwango (% / mwezi)</span>
                <input
                  type="number"
                  min="0.01"
                  step="0.01"
                  value={form.rate}
                  onChange={(e) => setForm({ ...form, rate: e.target.value })}
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm"
                />
              </label>
              <label className="block">
                <span className="mb-1 block text-xs text-muted-foreground">Aina ya riba</span>
                <select
                  value={form.itype}
                  onChange={(e) => setForm({ ...form, itype: e.target.value as "flat" | "reducing" })}
                  className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="flat">Flat</option>
                  <option value="reducing">Salio linalopungua</option>
                </select>
              </label>
            </div>
          )}
          <div className="grid grid-cols-2 gap-2">
            <label className="block">
              <span className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                <CalendarRange className="h-3 w-3" /> Muda wa chini (siku)
              </span>
              <input
                type="number"
                min="1"
                value={form.minDays}
                onChange={(e) => setForm({ ...form, minDays: e.target.value })}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-muted-foreground">Muda wa juu (siku)</span>
              <input
                type="number"
                min="1"
                value={form.maxDays}
                onChange={(e) => setForm({ ...form, maxDays: e.target.value })}
                className="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm"
              />
            </label>
          </div>
          <button
            onClick={() => proposeMutation.mutate()}
            disabled={proposeMutation.isPending}
            className="w-full rounded-xl bg-primary py-2.5 text-sm font-semibold text-primary-foreground disabled:opacity-50"
          >
            {proposeMutation.isPending ? "Inatuma…" : "Tuma pendekezo kwa Katibu"}
          </button>
        </div>
      )}
    </div>
  );
}
