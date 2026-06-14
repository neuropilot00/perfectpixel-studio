import { useState } from "react";
import { Loader2, Sparkles, ImagePlus, Wand2 } from "lucide-react";
import { PickImage, GenerateEdit } from "../../wailsjs/go/main/App";
import { STYLE_OPTIONS } from "../types";
import { useI18n } from "../i18n";
import { styleLabel } from "../i18n/catalog";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

interface IProps {
  baseImage: string | null;
  onClose: () => void;
  onToast: (kind: "info" | "error" | "success", text: string) => void;
  onUse: (image: string, name?: string) => void;
}

// 인페인팅/편집: 기존 이미지에 지시한 변경만 적용 (옷 바꾸기·악세서리 추가 등)
export default function EditModal({ baseImage, onClose, onToast, onUse }: IProps) {
  const { t, lang } = useI18n();
  const [img, setImg] = useState<string | null>(baseImage);
  const [instruction, setInstruction] = useState("");
  const [styleKey, setStyleKey] = useState("pixel");
  const [transparent, setTransparent] = useState(true);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  const pick = async () => {
    try {
      const d = await PickImage();
      if (d) { setImg(d); setResult(null); }
    } catch (e) {
      onToast("error", String(e));
    }
  };

  const run = async () => {
    if (!img || !instruction.trim() || busy) return;
    setBusy(true);
    setResult(null);
    try {
      const out = (await GenerateEdit({ image: img, instruction, styleKey, styleCustom: "", transparent } as any)) as string;
      if (out) { setResult(out); onToast("success", t("asset_saved_gallery")); }
    } catch (e) {
      onToast("error", String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="w-[560px]">
        <DialogTitle><Wand2 size={16} className="inline mr-1" /> {t("edit_studio")}</DialogTitle>
        <p className="hint">{t("edit_note")}</p>

        <div className="row" style={{ gap: 10, alignItems: "flex-start" }}>
          <div style={{ flex: 1 }}>
            <Label>{t("edit_source")}</Label>
            <div className="dropzone checker" style={{ minHeight: 130, cursor: "pointer" }} onClick={pick}>
              {img ? <img src={img} alt="src" className="pixelated" style={{ maxHeight: 150, objectFit: "contain" }} />
                   : <div className="dz-text"><ImagePlus size={22} /><br />{t("edit_pick")}</div>}
            </div>
          </div>
          {result && (
            <div style={{ flex: 1 }}>
              <Label>{t("edit_after")}</Label>
              <div className="checker" style={{ minHeight: 130, borderRadius: 10, padding: 6, display: "flex", justifyContent: "center" }}>
                <img src={result} alt="after" className="pixelated" style={{ maxHeight: 150, objectFit: "contain" }} />
              </div>
              <Button variant="outline" size="sm" style={{ width: "100%", marginTop: 4 }} onClick={() => onUse(result)}>
                {t("use_char_btn")}
              </Button>
            </div>
          )}
        </div>

        <div className="field">
          <Label>{t("edit_instruction")}</Label>
          <Input placeholder={t("edit_instruction_ph")} value={instruction} onChange={(e) => setInstruction(e.target.value)} onKeyDown={(e) => e.key === "Enter" && run()} />
        </div>

        <div className="row" style={{ gap: 12, alignItems: "flex-end" }}>
          <div style={{ flex: 1 }}>
            <Label>{t("art_style")}</Label>
            <Select value={styleKey} onValueChange={setStyleKey}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {STYLE_OPTIONS.map((o) => <SelectItem key={o.key} value={o.key}>{styleLabel(o.key, lang, o.label)}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 13, paddingBottom: 6 }}>
            <input type="checkbox" checked={transparent} onChange={(e) => setTransparent(e.target.checked)} />
            {t("edit_transparent")}
          </label>
        </div>

        <Button onClick={run} disabled={busy || !img || !instruction.trim()}>
          {busy ? <Loader2 size={13} className="animate-spin" /> : <Sparkles size={13} />}
          {busy ? t("edit_generating") : t("edit_generate")}
        </Button>

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose} disabled={busy}>{t("close")}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
