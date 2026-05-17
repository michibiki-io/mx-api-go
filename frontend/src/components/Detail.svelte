<script lang="ts">
  export let label: string;
  export let value: string;
  export let wide = false;
</script>

<div class={wide ? 'sm:col-span-2' : ''}>
  <div class="text-xs font-medium uppercase text-slate-500 dark:text-slate-400">{label}</div>
  <div class="mt-1 break-words rounded border border-slate-200 bg-slate-50 px-2 py-1 text-slate-900 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100">{value || '-'}</div>
</div>
