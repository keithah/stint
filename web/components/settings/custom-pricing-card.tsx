"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save, Trash2 } from "lucide-react";
import { useState } from "react";
import { deleteCustomPricing, listCustomPricing, upsertCustomPricing } from "@/lib/api";
import { customPricingError } from "@/app/settings/custom-pricing-validation";

export function CustomPricingCard() {
  const client = useQueryClient();
  const customPricing = useQuery({ queryKey: ["custom-pricing"], queryFn: listCustomPricing, retry: false });
  const [priceModel, setPriceModel] = useState("opencode/big-pickle");
  const [priceInput, setPriceInput] = useState(0);
  const [priceOutput, setPriceOutput] = useState(0);
  const [priceCacheWrite, setPriceCacheWrite] = useState(0);
  const [priceCacheRead, setPriceCacheRead] = useState(0);
  const customPricingValidationError = customPricingError({
    model: priceModel,
    inputPerMillion: priceInput,
    outputPerMillion: priceOutput,
    cacheWritePerMillion: priceCacheWrite,
    cacheReadPerMillion: priceCacheRead
  });
  const canSaveCustomPricing = customPricingValidationError === null;
  const savePricing = useMutation({
    mutationFn: () =>
      upsertCustomPricing({
        model: priceModel.trim(),
        input_per_million_usd: priceInput,
        output_per_million_usd: priceOutput,
        cache_write_per_million_usd: priceCacheWrite,
        cache_read_per_million_usd: priceCacheRead
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["custom-pricing"] })
  });
  const removePricing = useMutation({
    mutationFn: deleteCustomPricing,
    onSuccess: () => client.invalidateQueries({ queryKey: ["custom-pricing"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium">Custom AI pricing</h2>
          <p className="mt-1 text-sm text-zinc-400">Register per-token prices for private or proxied models (e.g. opencode/big-pickle) the bundled price table does not cover. Prices are USD per million tokens and override the base table when summarizing usage.</p>
        </div>
        <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => savePricing.mutate()} disabled={savePricing.isPending || !canSaveCustomPricing}>
          <Save size={16} /> Save price
        </button>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-5">
        <label className="block md:col-span-1">
          <span className="text-sm text-zinc-400">Model id</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={priceModel} onChange={(event) => setPriceModel(event.target.value)} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Input $ / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} step="any" value={priceInput} onChange={(event) => setPriceInput(Math.max(0, Number(event.target.value)))} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Output $ / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} step="any" value={priceOutput} onChange={(event) => setPriceOutput(Math.max(0, Number(event.target.value)))} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Cache write $ / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} step="any" value={priceCacheWrite} onChange={(event) => setPriceCacheWrite(Math.max(0, Number(event.target.value)))} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Cache read $ / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} step="any" value={priceCacheRead} onChange={(event) => setPriceCacheRead(Math.max(0, Number(event.target.value)))} />
        </label>
      </div>
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(customPricing.data?.data ?? []).map((price) => (
          <div key={price.model} className="grid items-center gap-2 px-3 py-3 text-sm sm:grid-cols-[1fr_auto]">
            <div className="grid gap-1">
              <span className="font-medium text-zinc-100">{price.model}</span>
              <span className="text-zinc-500">In ${price.input_per_million_usd}/1M · Out ${price.output_per_million_usd}/1M · Cache write ${price.cache_write_per_million_usd}/1M · Cache read ${price.cache_read_per_million_usd}/1M</span>
            </div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:border-red-400 hover:text-red-300 disabled:opacity-60"
              onClick={() => {
                setPriceModel(price.model);
                setPriceInput(price.input_per_million_usd);
                setPriceOutput(price.output_per_million_usd);
                setPriceCacheWrite(price.cache_write_per_million_usd);
                setPriceCacheRead(price.cache_read_per_million_usd);
              }}
            >
              Edit
            </button>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:border-red-400 hover:text-red-300 disabled:opacity-60 sm:col-start-2"
              onClick={() => removePricing.mutate(price.model)}
              disabled={removePricing.isPending}
            >
              <Trash2 size={16} /> Remove
            </button>
          </div>
        ))}
        {customPricing.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No custom prices yet. Add one above for a model your provider does not price.</div> : null}
      </div>
      {customPricingValidationError ? <p className="mt-3 text-sm text-amber-300">{customPricingValidationError}</p> : null}
      {savePricing.error ? <p className="mt-3 text-sm text-red-300">{savePricing.error.message}</p> : null}
      {removePricing.error ? <p className="mt-3 text-sm text-red-300">{removePricing.error.message}</p> : null}
    </section>
  );
}
