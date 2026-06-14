import { useState } from "react";
import { Loader2, Sparkles, Image as ImageIcon } from "lucide-react";
import { GenerateAsset } from "../../wailsjs/go/main/App";
import { STYLE_OPTIONS } from "../types";
import { useI18n } from "../i18n";
import { styleLabel } from "../i18n/catalog";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

interface IProps {
  onClose: () => void;
  onToast: (kind: "info" | "error" | "success", text: string) => void;
}

type Kind = "background" | "tile" | "item";

// 배경/타일/아이템 등 비캐릭터 에셋 생성 스튜디오 (활성 프로바이더 사용, 결과는 갤러리에 자동 저장)
export default function AssetStudioModal({ onClose, onToast }: IProps) {
  const { t, lang } = useI18n();
  const [kind, setKind] = useState<Kind>("background");
  const [desc, setDesc] = useState("");
  const [styleKey, setStyleKey] = useState("pixel");
  const [styleCustom, setStyleCustom] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  const generate = async () => {
    if (!desc.trim() || busy) return;
    setBusy(true);
    setResult(null);
    try {
      const dataURL = await GenerateAsset({ kind, description: desc, styleKey, styleCustom } as any);
      if (dataURL) {
        setResult(dataURL);
        onToast("success", t("asset_saved_gallery"));
      }
    } catch (e) {
      onToast("error", String(e));
    } finally {
      setBusy(false);
    }
  };

  const placeholder =
    kind === "background"
      ? t("asset_ph_background")
      : kind === "tile"
        ? t("asset_ph_tile")
        : t("asset_ph_item");

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogTitle>
          <ImageIcon size={16} className="inline mr-1" /> {t("asset_studio")}
        </DialogTitle>

        <Tabs value={kind} onValueChange={(v) => { setKind(v as Kind); setResult(null); }}>
          <TabsList>
            <TabsTrigger value="background">{t("asset_kind_background")}</TabsTrigger>
            <TabsTrigger value="tile">{t("asset_kind_tile")}</TabsTrigger>
            <TabsTrigger value="item">{t("asset_kind_item")}</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="field">
          <Label>{t("asset_desc")}</Label>
          <Textarea placeholder={placeholder} value={desc} onChange={(e) => setDesc(e.target.value)} />
        </div>

        <div className="field">
          <Label>{t("art_style")}</Label>
          <Select value={styleKey} onValueChange={setStyleKey}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STYLE_OPTIONS.map((o) => (
                <SelectItem key={o.key} value={o.key}>
                  {styleLabel(o.key, lang, o.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {styleKey === "custom" && (
          <div className="field">
            <Label>{t("custom_style_label")}</Label>
            <Textarea placeholder={t("custom_style_ph")} value={styleCustom} onChange={(e) => setStyleCustom(e.target.value)} />
          </div>
        )}

        <Button onClick={generate} disabled={busy || !desc.trim()}>
          {busy ? <Loader2 size={13} className="animate-spin" /> : <Sparkles size={13} />}
          {busy ? t("asset_generating") : t("asset_generate")}
        </Button>

        {kind === "tile" && <p className="hint">{t("asset_tile_note")}</p>}

        {result && (
          <div className="field">
            <Label>{t("asset_result")}</Label>
            <div className="checker" style={{ borderRadius: 12, padding: 8, display: "flex", justifyContent: "center" }}>
              <img src={result} alt="asset" className="pixelated" style={{ maxWidth: "100%", maxHeight: 320, objectFit: "contain" }} />
            </div>
            <p className="hint">{t("asset_saved_gallery")}</p>
          </div>
        )}

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
