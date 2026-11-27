# Weergave en stijlen

**Instellingen → Weergave** bepaalt hoe het venster eruitziet: het thema, de tekstgrootte en de taal, de lettertypen, de kleuren, en stijlen die lettertypen en kleuren onder een naam bewaren.

## Lettertypen en kleuren

Eén lettertype is voor tekst: de interface en de berichten. Het andere is voor code: codeblokken, uitvoer van tools, diffs en de terminal. Een lettertype dat niet op deze computer staat, wordt vervangen door het eigen lettertype van de app.

Naast elk lettertype staat zijn **Gewicht**, van Licht tot Halfvet; Normaal is dat van de app. Koppen en vette tekst gaan mee met het gewicht van de tekst en blijven vetter dan dat, vette tekst in de terminal net zo met dat van de code. Een lettertype zonder het gekozen gewicht toont het dichtstbijzijnde dat het heeft. In `config.yaml` zijn de gewichten `ui.textWeight` en `ui.codeWeight`, elk 100 tot 900 in stappen van 100.

Kleuren worden apart ingesteld voor de lichte en de donkere weergave, en een wijziging is meteen te zien. Freeloop waarschuwt als tekst moeilijk leesbaar wordt. De kleuren staan in `colors.json` in de gegevensmap, die ook met de hand te bewerken is.

## Merktekens van aanbieders

Waarop een sessie draait, staat als een klein merkteken in de zijbalk en op het paneel van de sessie. Voor llama.cpp, OpenRouter en vLLM is dat het logo van het project, dat de makers andere apps laten tonen; voor de rest een kleur met twee letters. Onder **Merktekens van aanbieders** zijn de kleur te wijzigen, letters te typen of met **Afbeelding…** een eigen afbeelding te kiezen, bijvoorbeeld een logo waarvoor de maker apps eerst om toestemming laat vragen. De afbeelding wordt verkleind tot een kleine PNG en bewaard in `config.yaml`, alleen op deze computer.

## Stijlen

Een stijl bewaart de twee lettertypen en de kleuren van beide weergaven onder een naam, zodat wisselen tussen twee uiterlijken één stap is. Thema, tekstgrootte, de gewichten van de lettertypen en taal horen niet bij een stijl.

Freeloop wordt geleverd met zes stijlen, onder **Meegeleverd**: Niks Theme, Solarized, Nord, Gruvbox, Rosé Pine en Dracula. Een meegeleverde stijl wordt gekozen en als nieuwe opgeslagen zoals elke andere, maar nooit bijgewerkt of verwijderd: voor een eigen versie hem kiezen, naar wens aanpassen en als nieuwe stijl opslaan.

- **Als nieuwe opslaan…** slaat de lettertypen en kleuren op het scherm op als stijl.
- Een stijl kiezen onder **Stijl**, of "Stijl" typen in het opdrachtenpalet, om ernaar te wisselen.
- Na een wijziging van een lettertype of een kleur staat de stijl als gewijzigd. **Stijl bijwerken** houdt de wijziging in de stijl; **Terugdraaien** gaat terug naar wat de stijl bevat.
- Wegwisselen van lettertypen en kleuren die in geen stijl staan, vraagt eerst, omdat ze anders verloren gaan.

## Een stijl delen

Elke stijl is een bestand in de map `styles` in de gegevensmap, die de knop {{Weergeven in Finder|Weergeven in Verkenner|Weergeven in bestandsbeheerder}} opent. Om een stijl te delen, het bestand versturen. Om er een toe te voegen, het bestand in die map zetten: binnen een seconde staat de stijl in de lijst.

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

De naam van het bestand, in kleine letters, cijfers en streepjes, is de ID van de stijl. Een stijl bevat alleen namen van lettertypen en kleuren als hexwaarde of `rgb()`, dus een stijl van iemand anders kan niets van internet laden en niets anders veranderen dan het uiterlijk. Een waarde die geen van beide is, wordt gemeld in de instellingen en weggelaten.
