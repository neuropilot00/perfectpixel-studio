import { useState, useRef, useEffect } from "react";
import { Loader2, Send, Bot } from "lucide-react";
import { ChatPlan, GenerateCharacter, GenerateAsset } from "../../wailsjs/go/main/App";
import { uid } from "../types";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Textarea } from "./ui/textarea";

interface IProps {
  onClose: () => void;
}

interface Msg {
  id: string;
  role: "user" | "assistant";
  text: string;
  images?: string[];
  pending?: boolean;
}

// 내장 AI 챗봇: 자연어 요청 → (Claude 또는 Codex가) 계획 → codex로 에셋 생성
export default function ChatModal({ onClose }: IProps) {
  const { t } = useI18n();
  const [messages, setMessages] = useState<Msg[]>([
    { id: uid("m"), role: "assistant", text: t("ai_greeting") },
  ]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages]);

  const push = (m: Msg) => setMessages((prev) => [...prev, m]);
  const update = (id: string, patch: Partial<Msg>) =>
    setMessages((prev) => prev.map((m) => (m.id === id ? { ...m, ...patch } : m)));

  const send = async () => {
    const msg = input.trim();
    if (!msg || busy) return;
    setInput("");
    const history = messages.map((m) => `${m.role === "user" ? "User" : "Assistant"}: ${m.text}`).join("\n");
    push({ id: uid("m"), role: "user", text: msg });
    setBusy(true);
    try {
      const plan: any = await ChatPlan(history, msg);
      push({ id: uid("m"), role: "assistant", text: plan.reply || "…" });
      for (const asset of plan.assets ?? []) {
        const pid = uid("m");
        const label = asset.name || asset.type;
        push({ id: pid, role: "assistant", text: `🎨 ${label} ${t("ai_generating_asset")}`, pending: true });
        try {
          let url: string;
          if (asset.type === "character") {
            url = (await GenerateCharacter({
              description: asset.description,
              styleKey: asset.styleKey || "pixel",
              styleCustom: "",
            } as any)) as string;
          } else {
            url = (await GenerateAsset({
              kind: asset.type,
              description: asset.description,
              styleKey: asset.styleKey || "pixel",
              styleCustom: "",
            } as any)) as string;
          }
          update(pid, { text: `✅ ${label}`, images: url ? [url] : [], pending: false });
        } catch (e) {
          update(pid, { text: `❌ ${label}: ${String(e)}`, pending: false });
        }
      }
    } catch (e) {
      push({ id: uid("m"), role: "assistant", text: `⚠️ ${String(e)}` });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="w-[560px]">
        <DialogTitle>
          <Bot size={16} className="inline mr-1" /> {t("ai_studio")}
        </DialogTitle>

        <div
          ref={scrollRef}
          style={{ maxHeight: 420, overflowY: "auto", display: "flex", flexDirection: "column", gap: 10, padding: "4px 2px" }}
        >
          {messages.map((m) => (
            <div
              key={m.id}
              style={{
                alignSelf: m.role === "user" ? "flex-end" : "flex-start",
                maxWidth: "85%",
                background: m.role === "user" ? "hsl(var(--primary))" : "hsl(var(--muted))",
                color: m.role === "user" ? "hsl(var(--primary-foreground))" : "inherit",
                borderRadius: 12,
                padding: "8px 12px",
                fontSize: 14,
                whiteSpace: "pre-wrap",
              }}
            >
              <span>
                {m.pending && <Loader2 size={12} className="animate-spin" style={{ display: "inline", marginRight: 4 }} />}
                {m.text}
              </span>
              {m.images && m.images.length > 0 && (
                <div className="checker" style={{ marginTop: 6, borderRadius: 8, padding: 6, display: "flex", gap: 6, flexWrap: "wrap" }}>
                  {m.images.map((src, i) => (
                    <img key={i} src={src} alt="" className="pixelated" style={{ maxWidth: 200, maxHeight: 200, objectFit: "contain" }} />
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>

        <div className="row" style={{ gap: 8, alignItems: "flex-end" }}>
          <Textarea
            className="flex-1"
            rows={2}
            placeholder={t("ai_placeholder")}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send();
              }
            }}
            disabled={busy}
          />
          <Button onClick={send} disabled={busy || !input.trim()}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          </Button>
        </div>
        <p className="hint">{t("ai_note")}</p>

        <div className="row justify-end">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t("close")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
