<template>
  <router-view />
</template>

<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Noto+Sans+SC:wght@400;500;600;700&display=swap');

:root {
  --bg: #0c0c0c;
  --bg-2: #141414;
  --bg-3: #1c1c1c;
  --bg-4: #252525;
  --text: #e8e8e8;
  --text-2: #adadad;
  --text-3: #888;
  --accent: #e85d3a;
  --accent-2: #ff7a57;
  --green: #3dd68c;
  --border: #2a2a2a;
  --radius: 6px;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: 'Noto Sans SC', sans-serif;
  background: var(--bg);
  color: var(--text);
}

::-webkit-scrollbar { width: 5px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--border); border-radius: 3px; }

input, textarea, button { font-family: inherit; }
</style>
