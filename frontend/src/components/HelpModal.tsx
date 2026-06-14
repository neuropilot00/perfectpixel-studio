import { Bot, Users, Wand2, Palette, Images, Package } from "lucide-react";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Label } from "./ui/label";

interface IProps {
  onClose: () => void;
}

// 인앱 도움말: 워크플로우 + 각 버튼 설명(기존 _tip 번역 재사용) + 팁
export default function HelpModal({ onClose }: IProps) {
  const { t } = useI18n();
  const tools = [
    { Icon: Bot, label: t("ai_studio"), desc: t("ai_studio_tip") },
    { Icon: Users, label: t("variants_studio"), desc: t("variants_tip") },
    { Icon: Wand2, label: t("edit_studio"), desc: t("edit_tip") },
    { Icon: Palette, label: t("asset_studio"), desc: t("asset_studio_tip") },
    { Icon: Images, label: t("gallery"), desc: t("gallery_tip") },
    { Icon: Package, label: t("export"), desc: t("export_tip") },
  ];
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="w-[560px]">
        <DialogTitle>❓ {t("help_title")}</DialogTitle>

        <div className="field">
          <Label>{t("help_workflow_title")}</Label>
          <p className="hint" style={{ fontSize: 13 }}>{t("onboard_steps")}</p>
        </div>

        <div className="field">
          <Label>{t("help_tools_title")}</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 6, marginTop: 4 }}>
            {tools.map((x, i) => (
              <div key={i} style={{ display: "flex", gap: 8, alignItems: "flex-start", fontSize: 13 }}>
                <x.Icon size={15} style={{ marginTop: 2, flexShrink: 0 }} />
                <span><b>{x.label}</b> — {x.desc}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="field">
          <Label>{t("help_tips_title")}</Label>
          <ul style={{ margin: "4px 0 0", paddingLeft: 18, fontSize: 13, lineHeight: 1.7, color: "hsl(var(--muted-foreground))" }}>
            <li>{t("help_tip_codex")}</li>
            <li>{t("help_tip_time")}</li>
            <li>{t("help_tip_frames")}</li>
            <li>{t("help_tip_use")}</li>
          </ul>
        </div>

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose}>{t("close")}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
