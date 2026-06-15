import { useState, useEffect, useRef } from "react";
import { Loader2, Play, ImagePlus, PersonStanding } from "lucide-react";
import { PickImage, RigAnimate } from "../../wailsjs/go/main/App";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

interface IProps {
  baseImage: string | null;
  onClose: () => void;
  onToast: (kind: "info" | "error" | "success", text: string) => void;
}

const ANIMS = ["walk", "run", "idle"];

// 스켈레톤 리그 애니메이션(베타): 캐릭터 1장 → 자동 리깅 → 걷기/달리기/대기 프레임 + 애니 미리보기
export default function RigModal({ baseImage, onClose, onToast }: IProps) {
  const { t } = useI18n();
  const [img, setImg] = useState<string | null>(baseImage);
  const [anim, setAnim] = useState("walk");
  const [frames, setFrames] = useState(8);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string[]>([]);
  const [sheet, setSheet] = useState<string | null>(null);
  const [idx, setIdx] = useState(0);
  const timer = useRef<number | null>(null);

  // 결과 프레임을 순환 재생
  useEffect(() => {
    if (result.length === 0) return;
    const fps = anim === "run" ? 14 : anim === "idle" ? 6 : 12;
    timer.current = window.setInterval(() => setIdx((i) => (i + 1) % result.length), 1000 / fps);
    return () => {
      if (timer.current) window.clearInterval(timer.current);
    };
  }, [result, anim]);

  const pick = async () => {
    try {
      const d = await PickImage();
      if (d) {
        setImg(d);
        setResult([]);
        setSheet(null);
      }
    } catch (e) {
      onToast("error", String(e));
    }
  };

  const run = async () => {
    if (!img || busy) return;
    setBusy(true);
    setResult([]);
    setSheet(null);
    try {
      const r: any = await RigAnimate({ image: img, anim, frames } as any);
      setResult(r.frames ?? []);
      setSheet(r.sheet ?? null);
      setIdx(0);
      onToast("success", t("rig_done"));
    } catch (e) {
      onToast("error", String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="w-[560px]">
        <DialogTitle>
          <PersonStanding size={16} className="inline mr-1" /> {t("rig_studio")}
        </DialogTitle>
        <p className="hint">{t("rig_note")}</p>

        <div className="row" style={{ gap: 10, alignItems: "flex-start" }}>
          <div style={{ flex: 1 }}>
            <Label>{t("edit_source")}</Label>
            <div className="dropzone checker" style={{ minHeight: 130, cursor: "pointer" }} onClick={pick}>
              {img ? (
                <img src={img} alt="src" className="pixelated" style={{ maxHeight: 150, objectFit: "contain" }} />
              ) : (
                <div className="dz-text"><ImagePlus size={22} /><br />{t("rig_pick")}</div>
              )}
            </div>
          </div>
          {result.length > 0 && (
            <div style={{ flex: 1 }}>
              <Label>{t("rig_preview")}</Label>
              <div className="checker" style={{ minHeight: 130, borderRadius: 10, padding: 6, display: "flex", justifyContent: "center", alignItems: "center" }}>
                <img src={result[idx]} alt="anim" className="pixelated" style={{ maxHeight: 150, objectFit: "contain" }} />
              </div>
              <p className="hint">{idx + 1}/{result.length} · {t("asset_saved_gallery")}</p>
            </div>
          )}
        </div>

        <div className="row" style={{ gap: 12, alignItems: "flex-end" }}>
          <div style={{ flex: 1 }}>
            <Label>{t("rig_anim")}</Label>
            <Select value={anim} onValueChange={(v) => { setAnim(v); setResult([]); }}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {ANIMS.map((a) => (
                  <SelectItem key={a} value={a}>{t("rig_anim_" + a)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div style={{ width: 110 }}>
            <Label>{t("frames_count")}</Label>
            <Input type="number" min={2} max={16} value={frames} onChange={(e) => setFrames(Math.max(2, Math.min(16, Number(e.target.value) || 8)))} />
          </div>
        </div>

        <Button onClick={run} disabled={busy || !img}>
          {busy ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}
          {busy ? t("rig_generating") : t("rig_generate")}
        </Button>

        {sheet && (
          <div className="field">
            <Label>{t("rig_sheet")}</Label>
            <div className="checker" style={{ borderRadius: 8, padding: 6, overflowX: "auto" }}>
              <img src={sheet} alt="sheet" className="pixelated" style={{ height: 90 }} />
            </div>
          </div>
        )}

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("close")}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
