import { useState } from "react";
import { Loader2, Sparkles, ImagePlus, RefreshCw, Users } from "lucide-react";
import { PickImage, GenerateCharacterRef } from "../../wailsjs/go/main/App";
import { STYLE_OPTIONS, uid } from "../types";
import { useI18n } from "../i18n";
import { styleLabel } from "../i18n/catalog";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Textarea } from "./ui/textarea";

interface IProps {
  baseImage: string | null; // 현재 작업 중인 베이스 캐릭터(있으면 레퍼런스 후보)
  onClose: () => void;
  onToast: (kind: "info" | "error" | "success", text: string) => void;
  onUse: (image: string, name?: string) => void; // 이 캐릭터로 작업(메인 파이프라인 로드)
}

interface Variant {
  id: string;
  desc: string;
  image?: string;
  status: "idle" | "busy" | "done" | "error";
  error?: string;
}

// 레퍼런스 기반 일괄 캐릭터 생성: 1개 화풍 레퍼런스 → 여러 캐릭터를 같은 스타일로 생성
export default function VariantsModal({ baseImage, onClose, onToast, onUse }: IProps) {
  const { t, lang } = useI18n();
  const [ref, setRef] = useState<string | null>(baseImage);
  const [descs, setDescs] = useState("");
  const [styleKey, setStyleKey] = useState("pixel");
  const [busy, setBusy] = useState(false);
  const [variants, setVariants] = useState<Variant[]>([]);

  const pickRef = async () => {
    try {
      const d = await PickImage();
      if (d) setRef(d);
    } catch (e) {
      onToast("error", String(e));
    }
  };

  const genOne = async (v: Variant) => {
    if (!ref) return;
    setVariants((prev) => prev.map((x) => (x.id === v.id ? { ...x, status: "busy", error: undefined } : x)));
    try {
      const url = (await GenerateCharacterRef({
        referenceImage: ref,
        description: v.desc,
        styleKey,
        styleCustom: "",
      } as any)) as string;
      setVariants((prev) => prev.map((x) => (x.id === v.id ? { ...x, status: "done", image: url } : x)));
    } catch (e) {
      setVariants((prev) => prev.map((x) => (x.id === v.id ? { ...x, status: "error", error: String(e) } : x)));
    }
  };

  const generateAll = async () => {
    if (!ref || busy) return;
    const lines = descs.split("\n").map((l) => l.trim()).filter(Boolean);
    if (lines.length === 0) {
      onToast("error", t("variants_need_desc"));
      return;
    }
    const list: Variant[] = lines.map((desc) => ({ id: uid("v"), desc, status: "idle" }));
    setVariants(list);
    setBusy(true);
    for (const v of list) {
      await genOne(v);
    }
    setBusy(false);
    onToast("success", t("variants_done", { n: lines.length }));
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="w-[620px]">
        <DialogTitle>
          <Users size={16} className="inline mr-1" /> {t("variants_studio")}
        </DialogTitle>
        <p className="hint">{t("variants_note")}</p>

        <div className="field">
          <Label>{t("variants_ref")}</Label>
          <div
            className="dropzone checker"
            style={{ minHeight: 120, cursor: "pointer" }}
            onClick={pickRef}
          >
            {ref ? (
              <img src={ref} alt="ref" className="pixelated" style={{ maxHeight: 140, objectFit: "contain" }} />
            ) : (
              <div className="dz-text"><ImagePlus size={24} /><br />{t("variants_pick_ref")}</div>
            )}
          </div>
        </div>

        <div className="field">
          <Label>{t("variants_descs")}</Label>
          <Textarea rows={4} placeholder={t("variants_descs_ph")} value={descs} onChange={(e) => setDescs(e.target.value)} />
        </div>

        <div className="field">
          <Label>{t("art_style")}</Label>
          <Select value={styleKey} onValueChange={setStyleKey}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              {STYLE_OPTIONS.map((o) => (
                <SelectItem key={o.key} value={o.key}>{styleLabel(o.key, lang, o.label)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Button onClick={generateAll} disabled={busy || !ref || !descs.trim()}>
          {busy ? <Loader2 size={13} className="animate-spin" /> : <Sparkles size={13} />}
          {busy ? t("variants_generating") : t("variants_generate")}
        </Button>

        {variants.length > 0 && (
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 8, marginTop: 4 }}>
            {variants.map((v) => (
              <div key={v.id} className="checker" style={{ borderRadius: 10, padding: 6, textAlign: "center", position: "relative" }}>
                {v.status === "busy" && <Loader2 size={20} className="animate-spin" />}
                {v.status === "done" && v.image && (
                  <>
                    <img src={v.image} alt={v.desc} className="pixelated" style={{ maxWidth: "100%", maxHeight: 130, objectFit: "contain" }} />
                    <Button variant="ghost" size="sm" style={{ position: "absolute", top: 2, right: 2 }} title={t("regenerate")} onClick={() => genOne(v)} disabled={busy}>
                      <RefreshCw size={12} />
                    </Button>
                    <Button variant="outline" size="sm" style={{ width: "100%", marginTop: 4 }} onClick={() => onUse(v.image!, v.desc)} disabled={busy}>
                      {t("use_char_btn")}
                    </Button>
                  </>
                )}
                {v.status === "error" && <span style={{ fontSize: 11, color: "hsl(var(--destructive))" }}>{t("variants_failed")}</span>}
                {v.status === "idle" && <span style={{ fontSize: 11, opacity: 0.6 }}>…</span>}
                <div style={{ fontSize: 10, opacity: 0.7, marginTop: 4, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{v.desc}</div>
              </div>
            ))}
          </div>
        )}

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("close")}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
