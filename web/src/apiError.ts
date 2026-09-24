export function explainDeviceIdentityError(msg: string): string {
  const m = (msg || "").trim();
  if (
    m.includes("нужен host или chassis MAC") ||
    m.includes("name и host обязательны") ||
    m.includes("этот MAC сейчас не виден на порту")
  ) {
    return [
      "Не удалось однозначно опознать устройство: на порту нет ни IP, ни MAC, который можно сохранить.",
      "",
      "Откуда NetLynx берёт данные на порту:",
      "• MAC — таблица FDB коммутатора (show mac address-table);",
      "• IP — ARP (часто со шлюза/L3). Камеры и МФУ часто не светятся в ARP;",
      "• LLDP может показать имя соседа, но chassis ID бывает не MAC.",
      "",
      "Что делать:",
      "1. Укажите имя и добавьте узел без адреса — IP и MAC можно вписать позже в карточке.",
      "2. Если знаете MAC (наклейка, DHCP, CLI свитча) — вставьте его в поле Host / IP: это тоже MAC.",
      "3. Дождитесь опроса FDB и снова откройте порт — когда MAC появится, линк на топологии запишется сам.",
    ].join("\n");
  }
  return m || "Ошибка запроса";
}

/** MessageBox с человеческим текстом (не сырой JSON). */
export function alertDeviceCreateError(err: unknown): void {
  const raw = err instanceof Error ? err.message : String(err);
  window.alert(explainDeviceIdentityError(raw));
}
