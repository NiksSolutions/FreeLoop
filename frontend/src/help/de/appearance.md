# Darstellung und Stile

**Einstellungen → Darstellung** legt fest, wie das Fenster aussieht: Farbmodus, Textgröße und Sprache, die Schriften, die Farben und Stile, die Schriften und Farben unter einem Namen festhalten.

## Schriften und Farben

Eine Schrift gilt für Text: die Oberfläche und die Nachrichten. Die andere gilt für Code: Codeblöcke, Ausgaben von Werkzeugen, Diffs und das Terminal. Eine Schrift, die auf diesem Computer fehlt, wird durch die eigene Schrift der App ersetzt.

Neben jeder Schrift steht ihre **Stärke**, von Leicht bis Halbfett; Normal ist die der App. Überschriften und fetter Text gehen mit der Stärke des Texts mit und bleiben fetter als sie, fetter Text im Terminal ebenso mit der des Codes. Eine Schrift ohne die gewählte Stärke zeigt die nächste, die sie hat. In `config.yaml` sind die Stärken `ui.textWeight` und `ui.codeWeight`, jeweils 100 bis 900 in Schritten von 100.

Farben werden für die helle und die dunkle Darstellung getrennt festgelegt, und eine Änderung ist sofort zu sehen. Freeloop warnt, wenn Text schwer lesbar wird. Die Farben stehen in `colors.json` im Datenordner, die sich auch von Hand bearbeiten lässt.

## Anbieterkennzeichen

Worauf eine Sitzung läuft, zeigt ein kleines Kennzeichen in der Seitenleiste und in ihrem Bereich. Für llama.cpp, OpenRouter und vLLM ist es das Logo des Projekts, das seine Macher anderen Apps zu zeigen erlauben; für die übrigen eine Farbe und zwei Buchstaben. Unter **Anbieterkennzeichen** lassen sich die Farbe ändern, Buchstaben eingeben oder mit **Bild…** ein eigenes Bild wählen. Für einen Anbieter oder Agenten, dessen Logo Freeloop nicht mitbringt, etwa Claude Code, OpenAI oder Gemini, holt **Offizielles Logo** es beim Klick von der Website seines Herstellers; nur dann verbindet sich Freeloop mit dieser Website, und nie im Modus Nur lokal. Das Bild wird zu einem kleinen PNG verkleinert und in `config.yaml` gespeichert, nur auf diesem Computer.

## Stile

Ein Stil hält die beiden Schriften und die Farben beider Darstellungen unter einem Namen fest, sodass ein Wechsel zwischen zwei Erscheinungsbildern ein Schritt ist. Farbmodus, Textgröße, die Stärken der Schriften und Sprache gehören nicht zu einem Stil.

Freeloop bringt sechs Stile mit, aufgeführt unter **Mitgeliefert**: Niks Theme, Solarized, Nord, Gruvbox, Rosé Pine und Dracula. Ein mitgelieferter Stil wird gewählt und als neuer gespeichert wie jeder andere, aber nie aktualisiert oder gelöscht: Für eine eigene Fassung ihn wählen, nach Belieben ändern und als neuen Stil speichern.

- **Als neuen speichern…** speichert die Schriften und Farben auf dem Bildschirm als Stil.
- Einen Stil unter **Stil** wählen oder „Stil“ in die Befehlspalette tippen, um zu ihm zu wechseln.
- Nach der Änderung einer Schrift oder Farbe gilt der Stil als geändert. **Stil aktualisieren** behält die Änderung im Stil; **Verwerfen** kehrt zu dem zurück, was der Stil enthält.
- Ein Wechsel weg von Schriften und Farben, die kein Stil enthält, fragt zuerst nach, da sie sonst verloren wären.

## Einen Stil weitergeben

Jeder Stil ist eine Datei im Ordner `styles` im Datenordner, den die Schaltfläche {{Im Finder zeigen|Im Explorer zeigen|Im Dateimanager zeigen}} öffnet. Zum Weitergeben eines Stils seine Datei verschicken. Zum Hinzufügen die Datei in diesen Ordner legen: Nach einer Sekunde steht der Stil in der Liste.

```
{
  "version": 1,
  "name": "Paper",
  "textFont": "Inter",
  "codeFont": "",
  "light": { "accent": "#7c3aed", "bg": "#fdfcf8" },
  "dark": {}
}
```

Der Name der Datei, aus Kleinbuchstaben, Ziffern und Bindestrichen, ist die ID des Stils. Ein Stil enthält nur Namen von Schriften und Farben als Hex-Wert oder `rgb()`, daher kann ein Stil von jemand anderem nichts aus dem Internet laden und nichts außer dem Aussehen ändern. Ein Wert, der keins von beidem ist, wird in den Einstellungen gemeldet und weggelassen.
