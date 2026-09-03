import { useEffect, useState } from "react";
import { Save, RotateCcw } from "lucide-react";
import { api, type Settings as SettingsT } from "../lib/api";
import { useToast } from "../components/toast";
import { Button, Card, Input, Field, Spinner } from "../components/ui";

export function Settings() {
  const toast = useToast();
  const [values, setValues] = useState<SettingsT | null>(null);
  const [initial, setInitial] = useState<SettingsT | null>(null);
  const [saving, setSaving] = useState(false);

  const load = () =>
    api
      .getSettings()
      .then((s) => {
        setValues(s);
        setInitial(s);
      })
      .catch((e) => toast.error(e.message));

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const dirty = values && initial && JSON.stringify(values) !== JSON.stringify(initial);

  const save = async () => {
    if (!values) return;
    setSaving(true);
    try {
      const updated = await api.updateSettings(values);
      setValues(updated);
      setInitial(updated);
      toast.success("Settings saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-muted">
          Monitoring behaviour, persisted server-side. Changes apply to the next poll cycle.
        </p>
      </div>

      {!values ? (
        <Card className="flex items-center gap-2 p-6 text-sm text-muted">
          <Spinner /> Loading settings…
        </Card>
      ) : (
        <Card className="space-y-5 p-4 sm:p-6">
          <Field label="Monitoring interval" hint="How often saved servers are polled. Minimum 15s.">
            <Input
              className="tnum"
              value={values.monitor_interval}
              onChange={(e) => setValues({ ...values, monitor_interval: e.target.value })}
              placeholder="5m"
            />
          </Field>
          <Field label="Measurement retention" hint="How long history is kept before pruning, e.g. 720h (30 days).">
            <Input
              className="tnum"
              value={values.retention}
              onChange={(e) => setValues({ ...values, retention: e.target.value })}
              placeholder="720h"
            />
          </Field>
          <Field
            label="Falseticker threshold"
            hint="A server whose offset differs from the median by more than this is flagged an outlier."
          >
            <Input
              className="tnum"
              value={values.outlier_threshold}
              onChange={(e) => setValues({ ...values, outlier_threshold: e.target.value })}
              placeholder="100ms"
            />
          </Field>

          <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={() => initial && setValues(initial)} disabled={!dirty || saving}>
              <RotateCcw className="h-4 w-4" /> Reset
            </Button>
            <Button onClick={save} disabled={!dirty || saving}>
              {saving ? <Spinner /> : <Save className="h-4 w-4" />} Save changes
            </Button>
          </div>
        </Card>
      )}

      <p className="text-xs text-muted">
        Durations use Go syntax: <span className="tnum">s</span>, <span className="tnum">m</span>,{" "}
        <span className="tnum">h</span> — for example <span className="tnum">30s</span>,{" "}
        <span className="tnum">5m</span>, <span className="tnum">720h</span>.
      </p>
    </div>
  );
}
