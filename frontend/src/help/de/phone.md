# Das Telefon

Sitzungen unterwegs vom Telefon aus verfolgen, über das eigene private Netz.

## Was das Telefon kann

- Live sehen, was wartet, und jede Sitzung.
- Eine Anfrage einmal erlauben oder ablehnen. Formulare, Pläne, Regeln und Einstellungen warten auf den Computer.
- Einer Sitzung eine Nachricht senden, auf Wunsch mit Bildern aus den Fotos oder eingefügt, oder ihre Runde anhalten.

Nachrichten und Entscheidungen vom Telefon sagen das in der Sitzung, mit dem Namen des Telefons.

## Einrichten

1. Unter **Einstellungen → Telefon** **Dem Telefon antworten** einschalten. Freeloop antwortet dann nur auf einem Port dieses Computers und läuft weiter, wenn das Fenster geschlossen wird.
2. Tailscale auf diesem Computer und dem Telefon installieren, beide mit demselben Konto anmelden und in der Tailscale-Verwaltung HTTPS-Zertifikate einschalten.
3. Den Befehl, den die Seite zeigt, einmal in einem Terminal ausführen. Er stellt Freeloop nur im eigenen Tailnet bereit. Die Seite zeigt auch, wie man ihn beendet.
4. **Telefon koppeln…** wählen, den angezeigten QR-Code mit der Kamera des Telefons scannen (oder die Adresse daneben auf dem Telefon öffnen) und auf dem Computer bestätigen. Danach auf dem Telefon einen Passkey hinzufügen, wenn es fragt: Anfragen beantworten und Nachrichten senden brauchen ihn.

Auf einem iPhone gibt **Zum Home-Bildschirm** in Safari ein eigenes Symbol; fragt es dort erneut nach dem Koppeln, einfach noch einmal koppeln.

## Erreichbar bleiben

- {{Der Mac muss wach bleiben. **Diesen Computer wach halten, solange eine Sitzung arbeitet oder wartet** verhindert den Ruhezustand bei Leerlauf; zugeklappt schläft er trotzdem, außer er hängt am Strom und an einem externen Bildschirm.|Der Computer muss wach bleiben. **Diesen Computer wach halten, solange eine Sitzung arbeitet oder wartet** verhindert den Ruhezustand bei Leerlauf; das Zuklappen kann ihn trotzdem schlafen legen.|Der Computer muss wach bleiben. **Diesen Computer wach halten, solange eine Sitzung arbeitet oder wartet** bittet das System, bei Leerlauf nicht zu schlafen; das Zuklappen folgt der eigenen Einstellung.}}
- Freeloop muss weiterlaufen: Das Schließen des Fensters blendet es aus, solange das Telefon an ist. Beenden hält es an.
- Tailscale muss verbunden bleiben, auf dem Computer und auf dem Telefon. Auf dem Computer dafür in Tailscale **Start on log in** einschalten, damit es nach einem Neustart wiederkommt. Erreicht das Telefon den Computer nicht, sagt es das.
- Der Nur-lokal-Modus schaltet das Telefon aus, da die Sitzungen sonst diesen Computer verließen.

## Was privat bleibt

- Freeloop antwortet nur auf diesem Computer. Tailscale bringt es verschlüsselt zum Telefon, und kein fremdes Gerät erreicht es.
- Nur gekoppelte und auf dem Computer bestätigte Telefone kommen hinein. **Alle Telefone abmelden** auf der Seite beendet sie alle, samt ihrer offenen Streams, sofort.
- Eine Anfrage vom Telefon beantworten oder eine Nachricht senden fragt 18 Stunden nach dem letzten Mal erneut nach dem Passkey des Telefons (Face ID, Touch ID oder der Code des Telefons). Wer ein entsperrtes Telefon in die Hände bekommt, kann Sitzungen lesen und anhalten, aber nicht lange in ihnen handeln. Ein Telefon, das vor den Passkeys gekoppelt wurde, wird gebeten, einen hinzuzufügen; **Einstellungen → Telefon** zeigt, welche Telefone einen haben, und das Entfernen eines Telefons entfernt seinen Passkey.
- Der Schlüssel eines Telefons liegt im eigenen Speicher der Telefon-App und wird als Header gesendet, nie als Cookie. Eine andere Seite unter demselben Tailscale-Namen erhält ihn daher nie, und er gilt nicht unter einem anderen Tailscale-Login oder nach 90 Tagen ohne Nutzung. Ein Telefon, das mit einem früheren Build gekoppelt wurde, muss neu koppeln.
- Eine Benachrichtigung läuft über den Push-Dienst deines Telefons (den von Apple, Google oder Mozilla), so verschlüsselt, dass nur dieses Telefon sie liest, und enthält nur, was geschehen ist, und die ID einer Sitzung. Der Push-Dienst erfährt, dass eine Nachricht gesendet wurde und wann. Der Nur-lokal-Modus sendet keine.
- Befehle, die Agenten ausführen, erreichen den Port wie jeden auf diesem Computer, bekommen dort aber ohne gekoppeltes Telefon nichts.
- Tailscale listet HTTPS-Zertifikate öffentlich, daher wird der Tailscale-Name des Computers öffentlich.
