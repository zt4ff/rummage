/**
 * Convert a cron expression to a friendly human-readable string.
 * Uses a simple mapping for common patterns; falls back to the raw cron
 * if the pattern isn't recognized.
 */
export function cronToFriendly(cron: string): string {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return cron;

  const [min, hour, dom, month, dow] = parts;

  // Every minute
  if (min === "*" && hour === "*" && dom === "*" && month === "*" && dow === "*") {
    return "Every minute";
  }

  // Every hour at :mm
  if (min !== "*" && hour === "*" && dom === "*" && month === "*" && dow === "*") {
    return `Every hour at :${min.padStart(2, "0")}`;
  }

  // Daily at HH:MM
  if (dom === "*" && month === "*" && dow === "*") {
    const h = parseInt(hour, 10);
    const suffix = h >= 12 ? "PM" : "AM";
    const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
    return `Daily at ${h12}:${min.padStart(2, "0")} ${suffix}`;
  }

  // Weekly on DOW at HH:MM
  if (dom === "*" && month === "*" && dow !== "*") {
    const dayNames = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
    const dayIdx = parseInt(dow, 10);
    const dayName = dayNames[dayIdx] || dow;
    const h = parseInt(hour, 10);
    const suffix = h >= 12 ? "PM" : "AM";
    const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
    const timeStr = h === 0 && min === "0" ? "midnight" : h === 12 && min === "0" ? "noon" : `${h12}:${min.padStart(2, "0")} ${suffix}`;
    return `Every ${dayName} at ${timeStr}`;
  }

  // Monthly on the dom at HH:MM
  if (month === "*" && dow === "*") {
    const h = parseInt(hour, 10);
    const suffix = h >= 12 ? "PM" : "AM";
    const h12 = h === 0 ? 12 : h > 12 ? h - 12 : h;
    return `Monthly on the ${dom}${ordinalSuffix(parseInt(dom, 10))} at ${h12}:${min.padStart(2, "0")} ${suffix}`;
  }

  return cron;
}

function ordinalSuffix(n: number): string {
  const s = ["th", "st", "nd", "rd"];
  const v = n % 100;
  return s[(v - 20) % 10] || s[v] || s[0];
}
