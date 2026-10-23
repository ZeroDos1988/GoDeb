import { useTranslation } from "react-i18next";
import { LineChart } from "@/components/LineChart.tsx";
import { StatusPill } from "@/components/Metrics.tsx";
import { useGet } from "@/lib/useAsync.ts";
import { EventList, latest, pick } from "./SystemTab.tsx";
import { Card, toMarkers, useEvents, useSeries, useTimeFormat, type RangeName, type Summary } from "./shared.tsx";

type Game = { id: string; name: string; ready: boolean; service: string };

export function GamesTab({ summary, range }: { summary: Summary | null; range: RangeName }) {
  const { t } = useTranslation();
  const games = useGet<{ games: Game[] }>("/api/plugins/steam/games");
  const list = (games.data?.games ?? []).filter((g) => g.ready);

  if (games.error) return <p className="text-sm text-white/60">{t("monitor.games.unavailable")}</p>;
  if (games.data && list.length === 0) return <p className="text-sm text-white/60">{t("monitor.games.none")}</p>;
  return (
    <div className="space-y-6">
      {list.map((g) => (
        <GameCard key={g.id} game={g} summary={summary} range={range} />
      ))}
    </div>
  );
}

function GameCard({ game, summary, range }: { game: Game; summary: Summary | null; range: RangeName }) {
  const { t } = useTranslation();
  const fmt = useTimeFormat(range);
  const events = useEvents(range);
  const markers = toMarkers(events, `steam:${game.id}`);
  const data = useSeries([`game:${game.id}:players`, `game:${game.id}:up`, `svc:game-${game.id}:*`], range);
  const d = data.data;
  const up = latest(summary, `game:${game.id}:up`) === 1;
  const players = latest(summary, `game:${game.id}:players`);
  const slots = latest(summary, `game:${game.id}:slots`);
  const common = {
    from: d?.from ?? 0,
    to: d?.to ?? 0,
    step: d?.step ?? 10,
    markers,
    formatTime: fmt.axis,
    emptyText: t("monitor.noData"),
  };
  const hasTraffic = Object.keys(d?.series ?? {}).some((k) => k.startsWith(`svc:game-${game.id}:`));

  return (
    <section className="space-y-3">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold">{game.name}</h2>
            <div className="mt-1 flex flex-wrap gap-2">
              <StatusPill tone={up ? "ok" : "idle"}>{up ? t("monitor.games.running") : t("monitor.games.stopped")}</StatusPill>
            </div>
          </div>
          <div className="text-right">
            <div className="text-xs tracking-wide text-white/60 uppercase">{t("monitor.games.players")}</div>
            <div className="text-2xl font-semibold">
              {players !== undefined ? players : "–"}
              {slots !== undefined ? <span className="text-base text-white/50"> / {slots}</span> : null}
            </div>
          </div>
        </div>
        {up && players === undefined ? <p className="mt-2 text-xs text-white/50">{t("monitor.games.noQuery")}</p> : null}
      </Card>
      <div className={`grid gap-4 lg:grid-cols-2 ${data.loading ? "opacity-60 transition-opacity" : ""}`}>
        <LineChart
          {...common}
          title={t("monitor.games.playersChart")}
          unit=""
          series={pick(d, [{ key: `game:${game.id}:players`, label: t("monitor.games.players") }])}
        />
        {hasTraffic ? (
          <LineChart
            {...common}
            title={t("monitor.games.traffic")}
            unit="B/s"
            series={pick(d, [
              { key: `svc:game-${game.id}:in`, label: t("monitor.rx") },
              { key: `svc:game-${game.id}:out`, label: t("monitor.tx") },
            ])}
          />
        ) : null}
      </div>
      <EventList range={range} markers={markers} formatTime={fmt.full} />
    </section>
  );
}
