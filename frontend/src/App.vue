<script setup>
import { ref, computed, onMounted } from "vue";
import {
  PawPrint,
  LayoutDashboard,
  ClipboardList,
  Scale,
  Cat,
  Box,
  Users,
  Settings,
  Plus,
  ChevronDown,
  ChevronRight,
  ArrowUpRight,
  Droplets,
  Search,
  Bell,
  LogOut,
  Globe,
  LockKeyhole,
  X,
  Check,
  Pencil,
  Trash2,
  Activity,
  Leaf,
  Heart,
  ArrowLeft,
  Menu,
  LoaderCircle,
  SlidersHorizontal,
  AlertCircle,
  CircleHelp,
  ExternalLink,
} from "lucide-vue-next";
import { api } from "./api";
import CatFace from "./components/CatFace.vue";
import WeightChart from "./components/WeightChart.vue";

const user = ref(null),
  isDemo = ref(false),
  demoAvailable = ref(false),
  booting = ref(true),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  toast = ref(""),
  authMode = ref("login"),
  auth = ref({ username: "", password: "", display_name: "" });
const tables = ref([]),
  publicTables = ref([]),
  dataset = ref(null),
  members = ref([]),
  tab = ref("dashboard"),
  selectedCat = ref(0),
  recordKind = ref("all"),
  range = ref(7),
  search = ref(""),
  modal = ref(""),
  form = ref({}),
  mobileNav = ref(false),
  tableMenu = ref(false),
  inviteName = ref(""),
  inviteRole = ref("editor"),
  publicSearch = ref("");
const nav = [
  { id: "dashboard", name: "生活概览", icon: LayoutDashboard },
  { id: "records", name: "屎尿记录", icon: ClipboardList },
  { id: "weights", name: "体重管理", icon: Scale },
  { id: "cats", name: "猫咪档案", icon: Cat },
  { id: "boxes", name: "猫砂盆与猫砂", icon: Box },
  { id: "share", name: "共享协作", icon: Users },
];
const titles = {
  dashboard: "生活概览",
  records: "屎尿记录",
  weights: "体重管理",
  cats: "猫咪档案",
  boxes: "猫砂盆与猫砂",
  share: "共享协作",
  settings: "表格设置",
  explore: "发现公开表格",
};
const cats = computed(() => dataset.value?.cats || []),
  activeCats = computed(() => cats.value.filter((c) => !c.archived)),
  boxes = computed(() => dataset.value?.boxes || []),
  litters = computed(() => dataset.value?.litters || []),
  records = computed(() => dataset.value?.records || []),
  weights = computed(() => dataset.value?.weights || []),
  report = computed(
    () =>
      dataset.value?.analysis || {
        alerts: [],
        matches: [],
        unknown: 0,
        confirmed: 0,
      },
  ),
  canWrite = computed(() => ["owner", "editor"].includes(dataset.value?.role)),
  isOwner = computed(() => dataset.value?.role === "owner");
const todayRecords = computed(() =>
    records.value.filter(
      (r) =>
        new Date(r.occurred_at).toDateString() === new Date().toDateString(),
    ),
  ),
  alerts = computed(() =>
    report.value.alerts.filter(
      (a) =>
        !selectedCat.value ||
        a.cat_id === selectedCat.value ||
        a.cat_id === null,
    ),
  ),
  unknowns = computed(() =>
    records.value.filter(
      (r) => !r.cat_id && Date.now() - new Date(r.occurred_at) < 7 * 86400000,
    ),
  );
const filteredRecords = computed(() =>
  records.value
    .filter(
      (r) =>
        (!selectedCat.value || r.cat_id === selectedCat.value) &&
        (recordKind.value === "all" || r.kind === recordKind.value) &&
        (!range.value ||
          Date.now() - new Date(r.occurred_at) <= range.value * 86400000) &&
        (!search.value ||
          [r.notes, catName(r.cat_id), boxName(r.box_id), r.litter_name]
            .join(" ")
            .toLowerCase()
            .includes(search.value.toLowerCase())),
    )
    .sort((a, b) => new Date(b.occurred_at) - new Date(a.occurred_at)),
);
const filteredWeights = computed(() =>
  weights.value
    .filter((w) => !selectedCat.value || w.cat_id === selectedCat.value)
    .sort((a, b) => new Date(b.occurred_at) - new Date(a.occurred_at)),
);
const selectedBox = computed(() =>
  boxes.value.find((b) => b.id === Number(form.value.box_id)),
);
const eligibleCats = computed(() =>
  cats.value.filter((c) =>
    (form.value.eligible_cat_ids || selectedBox.value?.cat_ids || []).includes(
      c.id,
    ),
  ),
);
const localTime = () => {
  const d = new Date();
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
};
const dateInput = (s) => {
  const d = new Date(s);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
};
const catName = (id) => cats.value.find((c) => c.id === id)?.name || "未知归属",
  boxName = (id) => boxes.value.find((b) => b.id === id)?.name || "猫砂盆",
  litterName = (id) => litters.value.find((l) => l.id === id)?.name || "未配置",
  getCat = (id) => cats.value.find((c) => c.id === id);
const stamp = (s) =>
  new Date(s).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
const latestWeight = (id) =>
  weights.value
    .filter((w) => w.cat_id === id)
    .sort((a, b) => new Date(b.occurred_at) - new Date(a.occurred_at))[0];
const catAlert = (id) => report.value.alerts.some((a) => a.cat_id === id);
const shapes = {
  pellet: "颗粒状",
  formed: "成形条状",
  soft: "软条状",
  paste: "糊状",
  watery: "水样",
};
const moisture = ["", "干硬", "偏干", "适中", "偏湿", "稀水"],
  sizes = ["", "很小", "偏小", "中等", "偏大", "很大"],
  loose = ["", "紧实", "较紧实", "适中", "偏松散", "易碎散"];
const materialNames = {
  tofu: "豆腐砂",
  bentonite: "膨润土",
  mixed: "混合砂",
  silica: "水晶砂",
  paper: "纸砂",
  other: "其他",
};
const matchFor = (r) => report.value.matches.find((m) => m.record_id === r.id);
const say = (msg) => {
  toast.value = msg;
  setTimeout(() => (toast.value = ""), 3500);
};
async function run(fn) {
  error.value = "";
  busy.value = true;
  try {
    await fn();
  } catch (e) {
    error.value = e.message;
    if (e.status === 401) {
      user.value = null;
      dataset.value = null;
    }
  } finally {
    busy.value = false;
  }
}
async function authenticate(mode) {
  await run(async () => {
    const body =
      mode === "demo"
        ? undefined
        : mode === "register"
          ? auth.value
          : { username: auth.value.username, password: auth.value.password };
    const res = await api("/auth/" + mode, "POST", body);
    user.value = res.user;
    await initialize();
    say(mode === "demo" ? "已进入本地演示，当前为示例数据" : "欢迎回来");
  });
}
async function initialize() {
  const me = await api("/me");
  user.value = me.user;
  isDemo.value = me.demo && me.user.username === "demo";
  tables.value = await api("/tables");
  if (tables.value.length) await loadTable(tables.value[0].id);
  else dataset.value = null;
}
async function loadTable(id) {
  loading.value = true;
  try {
    dataset.value = await api(`/tables/${id}/data`);
    selectedCat.value = 0;
    search.value = "";
    tableMenu.value = false;
    if (tab.value === "explore") tab.value = "dashboard";
    members.value = [];
  } finally {
    loading.value = false;
  }
}
async function switchTable(id) {
  await run(() => loadTable(id));
}
async function go(id) {
  tab.value = id;
  mobileNav.value = false;
  search.value = "";
  await run(async () => {
    if (id === "explore") publicTables.value = await api("/public");
    if (id === "share" && dataset.value && dataset.value.role !== "public")
      members.value = await api(`/tables/${dataset.value.table.id}/members`);
    if (id === "settings" && dataset.value)
      form.value = { ...dataset.value.table };
  });
}
async function refresh() {
  const id = dataset.value?.table.id;
  tables.value = await api("/tables");
  if (id) dataset.value = await api(`/tables/${id}/data`);
  if (tab.value === "share" && dataset.value.role !== "public")
    members.value = await api(`/tables/${id}/members`);
}
async function logout() {
  await run(async () => {
    await api("/auth/logout", "POST");
    user.value = null;
    dataset.value = null;
    isDemo.value = false;
    auth.value = { username: "", password: "", display_name: "" };
    tab.value = "dashboard";
  });
}
function openModal(type, item) {
  error.value = "";
  modal.value = type;
  const now = localTime();
  const defaults = {
    table: { name: "", description: "", visibility: "private" },
    cat: {
      name: "",
      breed: "",
      sex: "unknown",
      birthday: "",
      color: "sage",
      neutered: false,
      archived: false,
      notes: "",
    },
    litter: { name: "", material: "mixed", wait_minutes: 10, notes: "" },
    box: {
      name: "",
      location: "",
      litter_id: litters.value[0]?.id || "",
      cat_ids: activeCats.value.map((c) => c.id),
      archived: false,
    },
    weight: {
      cat_id: selectedCat.value || activeCats.value[0]?.id || "",
      kg: "",
      occurred_at: now,
      notes: "",
    },
    record: {
      kind: "urine",
      cat_id: selectedCat.value || "",
      box_id:
        boxes.value.find(
          (b) =>
            !b.archived &&
            (!selectedCat.value || b.cat_ids.includes(selectedCat.value)),
        )?.id || "",
      occurred_at: now,
      count: 1,
      grams: "",
      diameter_cm: "",
      looseness: 2,
      moisture: 3,
      shape: "formed",
      size: 3,
      body_weight_kg: "",
      blood: false,
      straining: false,
      no_output: false,
      notes: "",
    },
  };
  form.value = item
    ? {
        ...item,
        cat_ids: item.cat_ids ? [...item.cat_ids] : undefined,
        occurred_at: item.occurred_at ? dateInput(item.occurred_at) : undefined,
      }
    : { ...defaults[type] };
  if (item && type === "record")
    for (const key of [
      "grams",
      "diameter_cm",
      "looseness",
      "moisture",
      "size",
      "body_weight_kg",
      "cat_id",
    ])
      if (form.value[key] === null) form.value[key] = "";
}
function onBoxChange() {
  form.value.cat_id = "";
  form.value.eligible_cat_ids = undefined;
}
function onNoOutput() {
  form.value.count = form.value.no_output ? 0 : 1;
  if (form.value.no_output) {
    form.value.grams = "";
    form.value.diameter_cm = "";
  }
}
const num = (v) =>
  v === "" || v === null || v === undefined ? null : Number(v);
async function save() {
  await run(async () => {
    const f = form.value,
      type = modal.value,
      t = dataset.value?.table.id;
    let body,
      path,
      method = f.id ? "PATCH" : "POST";
    if (type === "table") {
      body = {
        name: f.name,
        description: f.description,
        visibility: f.visibility,
      };
      path = "/tables";
      method = "POST";
    }
    if (type === "cat") {
      body = {
        name: f.name,
        breed: f.breed,
        sex: f.sex,
        birthday: f.birthday,
        color: f.color,
        neutered: !!f.neutered,
        archived: !!f.archived,
        notes: f.notes,
      };
      path = `/tables/${t}/cats`;
    }
    if (type === "litter") {
      body = {
        name: f.name,
        material: f.material,
        wait_minutes: Number(f.wait_minutes),
        notes: f.notes,
      };
      path = `/tables/${t}/litters`;
    }
    if (type === "box") {
      body = {
        name: f.name,
        location: f.location,
        litter_id: Number(f.litter_id),
        cat_ids: f.cat_ids.map(Number),
        archived: !!f.archived,
      };
      path = `/tables/${t}/boxes`;
    }
    if (type === "weight") {
      body = {
        cat_id: Number(f.cat_id),
        kg: Number(f.kg),
        occurred_at: new Date(f.occurred_at).toISOString(),
        notes: f.notes,
      };
      path = `/tables/${t}/weights`;
    }
    if (type === "record") {
      body = {
        kind: f.kind,
        cat_id: num(f.cat_id),
        box_id: Number(f.box_id),
        occurred_at: new Date(f.occurred_at).toISOString(),
        count: Number(f.count),
        grams: num(f.grams),
        diameter_cm: f.kind === "urine" ? num(f.diameter_cm) : null,
        looseness: f.kind === "urine" && !f.no_output ? num(f.looseness) : null,
        moisture: f.kind === "stool" && !f.no_output ? num(f.moisture) : null,
        shape: f.kind === "stool" && !f.no_output ? f.shape : "",
        size: f.kind === "stool" && !f.no_output ? num(f.size) : null,
        body_weight_kg: num(f.body_weight_kg),
        blood: !!f.blood,
        straining: !!f.straining,
        no_output: !!f.no_output,
        notes: f.notes,
      };
      path = `/tables/${t}/records`;
    }
    const result = await api(
      path + (f.id && type !== "table" ? "/" + f.id : ""),
      method,
      body,
    );
    modal.value = "";
    if (type === "table") {
      tables.value = await api("/tables");
      await loadTable(result.id);
      tab.value = "dashboard";
    } else await refresh();
    say("已保存，家人们可以看到最新记录");
  });
}
async function remove(type, id) {
  if (!window.confirm("确定删除这条记录？删除后无法恢复。")) return;
  await run(async () => {
    await api(`/tables/${dataset.value.table.id}/${type}/${id}`, "DELETE");
    await refresh();
    say("记录已删除");
  });
}
async function saveSettings() {
  await run(async () => {
    const f = form.value;
    await api(`/tables/${dataset.value.table.id}`, "PATCH", {
      name: f.name,
      description: f.description,
      visibility: f.visibility,
    });
    await refresh();
    say("表格设置已更新");
  });
}
async function join() {
  await run(async () => {
    await api(`/tables/${dataset.value.table.id}/join`, "POST");
    await refresh();
    say("申请已发送，等待表主审核");
  });
}
async function invite() {
  await run(async () => {
    await api(`/tables/${dataset.value.table.id}/members`, "POST", {
      username: inviteName.value,
      role: inviteRole.value,
    });
    inviteName.value = "";
    await refresh();
    say("成员已加入协作");
  });
}
async function memberUpdate(m, status, role) {
  await run(async () => {
    await api(
      `/tables/${dataset.value.table.id}/members/${m.user_id}`,
      "PATCH",
      { role: role || m.role, status },
    );
    await refresh();
    say("成员权限已更新");
  });
}
async function findPublic() {
  await run(
    async () =>
      (publicTables.value = await api(
        "/public?q=" + encodeURIComponent(publicSearch.value),
      )),
  );
}
onMounted(async () => {
  try {
    demoAvailable.value = (await api("/capabilities")).demo;
    await initialize();
  } catch (e) {
    if (e.status !== 401) error.value = e.message;
  } finally {
    booting.value = false;
  }
});
</script>

<template>
  <div v-if="booting" class="boot">
    <PawPrint :size="34" /><span>正在打开猫猫手账…</span>
  </div>
  <div v-else-if="!user" class="auth-page">
    <div class="auth-story">
      <div class="brand">
        <span class="brand-mark"><PawPrint /></span>
        <div>猫猫手账<small>CATCARE JOURNAL</small></div>
      </div>
      <div class="auth-copy">
        <span class="eyebrow">FOR EVERY LITTLE CHANGE</span>
        <h1>小小的日常，<br />满满的在意。</h1>
        <p>
          从一团猫砂，到每一次称重。<br />和家人一起，把猫咪的健康写进生活。
        </p>
        <div class="auth-cats">
          <CatFace color="sage" /><CatFace color="peach" /><CatFace
            color="lavender"
            sleepy
          />
        </div>
        <div class="auth-feature">
          <span><Scale :size="17" />体重趋势</span
          ><span><Droplets :size="17" />屎尿观察</span
          ><span><Users :size="17" />家人协作</span>
        </div>
      </div>
      <p class="auth-foot">每一只猫，都值得被细心照顾。<Heart :size="15" /></p>
    </div>
    <div class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="authenticate(authMode)">
        <span class="eyebrow">WELCOME HOME</span>
        <h2>
          {{
            authMode === "login" ? "欢迎回到猫猫的小家" : "一起建一个猫咪小家"
          }}
        </h2>
        <p>用自己的账号，开始一份有温度的健康手账。</p>
        <div class="segmented">
          <button
            type="button"
            :class="{ active: authMode === 'login' }"
            @click="
              authMode = 'login';
              error = '';
            "
          >
            登录</button
          ><button
            type="button"
            :class="{ active: authMode === 'register' }"
            @click="
              authMode = 'register';
              error = '';
            "
          >
            注册
          </button>
        </div>
        <label v-if="authMode === 'register'"
          >你的昵称<input
            v-model="auth.display_name"
            required
            maxlength="40"
            autocomplete="nickname"
            placeholder="家人们怎么称呼你" /></label
        ><label
          >账号<input
            v-model="auth.username"
            required
            pattern="[a-zA-Z0-9_]{3,32}"
            autocomplete="username"
            placeholder="3–32位英文、数字或下划线" /></label
        ><label
          >密码<input
            v-model="auth.password"
            required
            :minlength="authMode === 'register' ? 8 : 1"
            maxlength="72"
            type="password"
            :autocomplete="
              authMode === 'register' ? 'new-password' : 'current-password'
            "
            placeholder="请输入密码"
        /></label>
        <div v-if="error" class="error" role="alert">{{ error }}</div>
        <button class="btn primary auth-submit" :disabled="busy">
          <LoaderCircle v-if="busy" class="spin" :size="18" />{{
            authMode === "login" ? "登录猫猫手账" : "创建账号"
          }}<ChevronRight :size="17" />
        </button>
        <div v-if="demoAvailable" class="auth-divider">
          <span>先看看长什么样</span>
        </div>
        <button
          type="button"
          v-if="demoAvailable"
          class="btn demo-btn"
          :disabled="busy"
          @click="authenticate('demo')"
        >
          <PawPrint :size="18" />进入本地演示</button
        ><small v-if="demoAvailable"
          >演示入口仅在开发环境启用，正式部署默认关闭。</small
        >
      </form>
    </div>
  </div>
  <div v-else class="app-shell">
    <div v-if="mobileNav" class="nav-scrim" @click="mobileNav = false" />
    <aside class="sidebar" :class="{ open: mobileNav }">
      <div class="brand">
        <span class="brand-mark"><PawPrint :size="24" /></span>
        <div>猫猫手账<small>CATCARE JOURNAL</small></div>
      </div>
      <div class="table-switch">
        <button class="table-switch-btn" @click="tableMenu = !tableMenu">
          <span class="table-icon"><ClipboardList :size="18" /></span
          ><span
            >{{ dataset?.table.name || "我的猫咪小家"
            }}<small>{{
              dataset?.table.visibility === "public" ? "公开表格" : "私有表格"
            }}</small></span
          ><ChevronDown :size="15" />
        </button>
        <div v-if="tableMenu" class="table-dropdown">
          <button v-for="t in tables" :key="t.id" @click="switchTable(t.id)">
            {{ t.name
            }}<Check v-if="t.id === dataset?.table.id" :size="14" /></button
          ><button
            @click="
              openModal('table');
              tableMenu = false;
            "
          >
            <Plus :size="15" />新建管理表格
          </button>
        </div>
      </div>
      <span class="nav-label">猫咪的日常</span>
      <nav>
        <button
          v-for="n in nav"
          :key="n.id"
          :class="{ active: tab === n.id }"
          @click="go(n.id)"
        >
          <component :is="n.icon" :size="19" />{{ n.name
          }}<span v-if="n.id === 'share'" class="nav-dot" />
        </button>
      </nav>
      <div class="nav-separator" />
      <button
        class="nav-extra"
        :class="{ active: tab === 'explore' }"
        @click="go('explore')"
      >
        <Globe :size="19" />发现公开表格<ArrowUpRight :size="14" /></button
      ><button
        class="nav-extra"
        :class="{ active: tab === 'settings' }"
        @click="go('settings')"
      >
        <Settings :size="19" />表格设置
      </button>
      <div class="sidebar-note">
        <Leaf :size="22" />
        <p>每天一点点记录，<br />多一份安心。</p>
        <span>把在意，留在日常里。</span>
      </div>
      <div class="profile">
        <span class="profile-avatar">{{ user.display_name.slice(0, 1) }}</span>
        <div>
          {{ user.display_name
          }}<small>{{
            isDemo ? "本地演示 · 示例数据" : "@" + user.username
          }}</small>
        </div>
        <button
          class="icon-btn"
          title="退出登录"
          aria-label="退出登录"
          @click="logout"
        >
          <LogOut :size="17" />
        </button>
      </div>
    </aside>
    <main class="main">
      <header class="topbar">
        <button
          class="icon-btn mobile-menu"
          @click="mobileNav = true"
          aria-label="打开菜单"
        >
          <Menu />
        </button>
        <div class="breadcrumbs">
          我的小家<ChevronRight :size="13" /><strong>{{ titles[tab] }}</strong
          ><span v-if="isDemo" class="demo-label">示例数据</span>
        </div>
        <div class="topbar-right">
          <span class="today">{{
            new Date().toLocaleDateString("zh-CN", {
              month: "long",
              day: "numeric",
              weekday: "short",
            })
          }}</span
          ><button
            class="icon-btn bell"
            aria-label="查看观察提醒"
            @click="
              go('dashboard');
              selectedCat = 0;
            "
          >
            <Bell :size="19" /><i v-if="report.alerts.length" />
          </button>
        </div>
      </header>
      <div class="content">
        <div class="page-heading">
          <div>
            <span class="eyebrow">{{
              tab === "dashboard"
                ? "LITTLE PAWS, EVERYDAY CARE"
                : "A SHARED JOURNAL OF CARE"
            }}</span>
            <h1>
              {{ tab === "dashboard" ? "把小日子，照顾好。" : titles[tab]
              }}<span v-if="tab === 'dashboard'" class="heading-paw">✳</span>
            </h1>
            <p>
              {{
                tab === "dashboard"
                  ? "猫咪不会说话，但每一个小变化都有迹可循。"
                  : "和家人一起记录，让每一次观察都变得有意义。"
              }}
            </p>
          </div>
          <button
            v-if="canWrite && ['dashboard', 'records'].includes(tab)"
            class="btn primary"
            @click="openModal('record')"
          >
            <Plus :size="18" />记一笔屎尿</button
          ><button
            v-if="canWrite && tab === 'weights'"
            class="btn primary"
            @click="openModal('weight')"
          >
            <Plus :size="18" />记录体重</button
          ><button
            v-if="canWrite && tab === 'cats'"
            class="btn primary"
            @click="openModal('cat')"
          >
            <Plus :size="18" />添加猫咪</button
          ><button
            v-if="canWrite && tab === 'boxes'"
            class="btn primary"
            @click="openModal('box')"
          >
            <Plus :size="18" />添加猫砂盆</button
          ><button
            v-if="tab === 'explore' || !dataset"
            class="btn primary"
            @click="openModal('table')"
          >
            <Plus :size="18" />新建表格
          </button>
        </div>
        <div v-if="error && !modal" class="error page-error" role="alert">
          {{ error
          }}<button class="icon-btn" @click="error = ''" aria-label="关闭错误">
            <X :size="16" />
          </button>
        </div>
        <div v-if="loading" class="loading">
          <LoaderCircle class="spin" />正在加载手账…
        </div>
        <template v-else-if="tab === 'explore'"
          ><div class="panel public-intro">
            <Globe :size="28" />
            <div>
              <h3>让照顾多一双手</h3>
              <p>
                公开表格登录后可浏览。申请协作通过后，即可按表主授予的权限共同管理。
              </p>
            </div>
          </div>
          <form class="public-search" @submit.prevent="findPublic">
            <Search :size="18" /><input
              v-model="publicSearch"
              placeholder="搜索公开表格"
            /><button class="btn secondary" :disabled="busy">搜索</button>
          </form>
          <div class="grid-two">
            <article
              class="panel public-card"
              v-for="t in publicTables"
              :key="t.id"
            >
              <span class="pill sage"><Globe :size="12" />公开</span>
              <h3>{{ t.name }}</h3>
              <p>{{ t.description || "一份公开的猫咪健康手账。" }}</p>
              <button class="text-btn" @click="switchTable(t.id)">
                打开表格<ArrowUpRight :size="16" />
              </button>
            </article>
          </div>
          <div v-if="!publicTables.length" class="empty">
            暂时没有符合条件的公开表格。
          </div></template
        >
        <div v-else-if="!dataset" class="panel empty welcome-empty">
          <CatFace color="sage" />
          <h2>给猫咪建一个小家吧</h2>
          <p>
            新建表格后，添加猫咪与猫砂盆，就能开始记录。<br />也可以去发现公开表格，申请和朋友共同管理。
          </p>
          <button class="btn primary" @click="openModal('table')">
            <Plus :size="17" />创建第一份手账
          </button>
        </div>
        <template v-else>
          <div v-if="dataset.role === 'public'" class="public-banner">
            <Globe :size="18" /><span
              >你正在浏览公开表格。加入协作后可按授权共同记录。</span
            ><button
              class="btn secondary small"
              :disabled="busy || dataset.join_status === 'pending'"
              @click="join"
            >
              {{
                dataset.join_status === "pending"
                  ? "等待表主审核"
                  : "申请加入协作"
              }}
            </button>
          </div>
          <template v-if="['dashboard', 'records', 'weights'].includes(tab)"
            ><div class="cat-filter">
              <button
                :class="{ active: !selectedCat }"
                @click="selectedCat = 0"
              >
                <PawPrint :size="15" />全部猫咪<span>{{
                  activeCats.length
                }}</span></button
              ><button
                v-for="c in activeCats"
                :key="c.id"
                :class="{ active: selectedCat === c.id }"
                @click="selectedCat = c.id"
              >
                <span class="color-dot" :class="c.color" />{{ c.name
                }}<i v-if="catAlert(c.id)" class="alert-dot" /></button
              ><button
                v-if="canWrite"
                class="add-cat"
                @click="openModal('cat')"
                aria-label="添加猫咪"
              >
                <Plus :size="17" />
              </button></div
          ></template>
          <template v-if="tab === 'dashboard'">
            <div class="stats-grid">
              <article class="stat-card">
                <span class="stat-label">家里的毛孩子<Cat :size="18" /></span>
                <div>{{ activeCats.length }}<small>只</small></div>
                <p>
                  在
                  {{
                    boxes.filter((b) => !b.archived).length
                  }}
                  个猫砂盆里安心生活
                </p>
              </article>
              <article class="stat-card">
                <span class="stat-label"
                  >今天的尿团<Droplets :size="18"
                /></span>
                <div>
                  {{
                    todayRecords
                      .filter((r) => r.kind === "urine")
                      .reduce((sum, r) => sum + r.count, 0)
                  }}<small>团</small>
                </div>
                <p>今日已记录 · 包含未知归属</p>
              </article>
              <article class="stat-card">
                <span class="stat-label"
                  >今天的大便<ClipboardList :size="18"
                /></span>
                <div>
                  {{
                    todayRecords
                      .filter((r) => r.kind === "stool")
                      .reduce((sum, r) => sum + r.count, 0)
                  }}<small>次 / 份</small>
                </div>
                <p>干湿 · 形态 · 大小，都有记录</p>
              </article>
              <article class="stat-card attention">
                <span class="stat-label"
                  >需要多看一眼<Activity :size="18"
                /></span>
                <div>{{ report.alerts.length }}<small>项</small></div>
                <p>
                  {{
                    report.alerts.length
                      ? "近期变化值得继续观察"
                      : "暂无触发阈值的提醒"
                  }}
                </p>
              </article>
            </div>
            <div class="cat-cards">
              <button
                v-for="c in activeCats.filter(
                  (c) => !selectedCat || c.id === selectedCat,
                )"
                :key="c.id"
                class="cat-card"
                @click="selectedCat = selectedCat === c.id ? 0 : c.id"
              >
                <CatFace :color="c.color" />
                <div class="cat-card-info">
                  <h3>
                    {{ c.name
                    }}<span>{{
                      c.sex === "male" ? "♂" : c.sex === "female" ? "♀" : ""
                    }}</span>
                  </h3>
                  <p>
                    {{ c.breed || "猫咪" }} ·
                    {{ c.neutered ? "已绝育" : "未标记绝育" }}
                  </p>
                  <span :class="['pill', catAlert(c.id) ? 'amber' : 'sage']"
                    ><i />{{
                      catAlert(c.id) ? "留意近期变化" : "持续观察中"
                    }}</span
                  >
                </div>
                <div class="cat-card-weight">
                  <strong>{{ latestWeight(c.id)?.kg.toFixed(2) || "—" }}</strong
                  ><span>kg · 最近体重</span>
                </div>
              </button>
            </div>
            <div class="dashboard-middle">
              <section class="panel chart-panel">
                <div class="panel-heading">
                  <div>
                    <h3>
                      体重的小曲线<span class="pill neutral">近30天</span>
                    </h3>
                    <p>稳定的小变化，是每天的安心。</p>
                  </div>
                  <button class="text-btn" @click="go('weights')">
                    查看全部<ArrowUpRight :size="15" />
                  </button>
                </div>
                <WeightChart
                  :weights="weights"
                  :cats="cats"
                  :cat-id="selectedCat"
                />
                <div class="chart-footer">
                  <span
                    ><Scale :size="14" />每次尽量在相同时间、使用同一台秤</span
                  ><button
                    v-if="canWrite"
                    class="text-btn"
                    @click="openModal('weight')"
                  >
                    <Plus :size="14" />记录体重
                  </button>
                </div>
              </section>
              <section class="panel shared-panel">
                <div class="panel-heading">
                  <h3>共享盆，也能细心观察</h3>
                  <span class="soft-icon"><Users :size="18" /></span>
                </div>
                <div class="shared-number">
                  <strong>{{ unknowns.length }}</strong
                  ><span
                    >条记录待确认归属<small
                      >最近7天 · 先记录，再核对</small
                    ></span
                  >
                </div>
                <div class="shared-meter">
                  <span
                    :style="{
                      width:
                        (report.confirmed /
                          (report.confirmed + report.unknown || 1)) *
                          100 +
                        '%',
                    }"
                  />
                </div>
                <div class="meter-label">
                  <span
                    ><i class="sage-dot" />{{ report.confirmed }} 条已确认</span
                  ><span
                    ><i class="amber-dot" />{{ report.unknown }} 条未知</span
                  >
                </div>
                <div class="shared-note">
                  <CircleHelp :size="17" />
                  <p>特征相似时保留未知；人工确认的记录才会进入单猫基线。</p>
                </div>
                <button
                  class="text-btn"
                  @click="
                    go('records');
                    selectedCat = 0;
                  "
                >
                  核对记录<ChevronRight :size="15" />
                </button>
              </section>
            </div>
            <section class="panel alerts-panel">
              <div class="panel-heading">
                <h3>
                  今天，多留意一点<span class="pill amber" v-if="alerts.length"
                    >{{ alerts.length }} 条观察</span
                  >
                </h3>
                <span class="subtle">根据近28天个人基线</span>
              </div>
              <div v-if="!alerts.length" class="quiet-alert">
                <Check :size="19" /><span
                  >暂无触发阈值的变化。样本不足时仍需继续记录与观察。</span
                >
              </div>
              <div
                v-for="(a, i) in alerts.slice(0, 4)"
                :key="i"
                class="alert-row"
                :class="a.level"
              >
                <span class="alert-icon"><Activity :size="19" /></span>
                <div>
                  <h4>
                    {{ a.cat_id ? catName(a.cat_id) : boxName(a.box_id) }} ·
                    {{ a.title
                    }}<span v-if="a.common" class="pill neutral"
                      >同盆共同变化</span
                    >
                  </h4>
                  <p>{{ a.detail }}</p>
                </div>
                <button
                  v-if="a.record_id"
                  class="text-btn"
                  @click="
                    go('records');
                    selectedCat = a.cat_id || 0;
                    range = 30;
                  "
                >
                  查看记录<ChevronRight :size="14" />
                </button>
              </div>
              <p class="health-note">
                统计提醒用于观察，不是诊断。排尿用力却无尿时，请立即联系兽医。
              </p>
            </section>
          </template>
          <section
            v-if="['dashboard', 'records'].includes(tab)"
            class="panel records-panel"
          >
            <div class="panel-heading">
              <div>
                <h3>
                  {{ tab === "dashboard" ? "最近的生活记录" : "每一笔，都算数"
                  }}<span class="count-label"
                    >{{ filteredRecords.length }} 条</span
                  >
                </h3>
                <p v-if="tab === 'records'">
                  未知归属的记录保留在盆级观察中，不混入单猫统计。
                </p>
              </div>
              <button
                v-if="tab === 'dashboard'"
                class="text-btn"
                @click="go('records')"
              >
                全部记录<ArrowUpRight :size="15" />
              </button>
            </div>
            <div class="table-toolbar">
              <div class="segmented compact">
                <button
                  v-for="[k, label] in [
                    ['all', '全部记录'],
                    ['urine', '尿团'],
                    ['stool', '大便'],
                  ]"
                  :key="k"
                  :class="{ active: recordKind === k }"
                  @click="recordKind = k"
                >
                  {{ label }}
                </button>
              </div>
              <div class="table-tools">
                <div class="search-field">
                  <Search :size="15" /><input
                    v-model="search"
                    aria-label="搜索记录"
                    placeholder="搜索猫咪、备注…"
                  />
                </div>
                <select v-model.number="range" aria-label="记录时间范围">
                  <option :value="7">最近7天</option>
                  <option :value="30">最近30天</option>
                  <option :value="0">全部时间</option>
                </select>
              </div>
            </div>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>记录时间</th>
                    <th>猫咪 / 归属</th>
                    <th>类型</th>
                    <th>量化观察</th>
                    <th>猫砂盆 / 猫砂</th>
                    <th>备注</th>
                    <th v-if="canWrite">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="r in tab === 'dashboard'
                      ? filteredRecords.slice(0, 5)
                      : filteredRecords"
                    :key="r.id"
                  >
                    <td class="date-cell">
                      {{ stamp(r.occurred_at)
                      }}<small
                        >{{ r.count }} {{ r.kind === "urine" ? "团" : "份"
                        }}{{ r.no_output ? " · 无产出" : "" }}</small
                      >
                    </td>
                    <td>
                      <span class="table-cat"
                        ><CatFace
                          v-if="r.cat_id"
                          :color="getCat(r.cat_id)?.color"
                        /><span v-else class="unknown-avatar"
                          ><CircleHelp :size="17" /></span
                        >{{ catName(r.cat_id) }}</span
                      ><small
                        v-if="!r.cat_id && matchFor(r)?.candidates.length"
                        class="match-hint"
                        :title="matchFor(r).reason"
                        >{{
                          matchFor(r).status === "review"
                            ? "建议核对"
                            : "特征重叠"
                        }}
                        · {{ catName(matchFor(r).candidates[0].cat_id) }}
                        {{ matchFor(r).candidates[0].score }}分</small
                      >
                    </td>
                    <td>
                      <span
                        :class="['pill', r.kind === 'urine' ? 'blue' : 'sand']"
                        ><Droplets
                          v-if="r.kind === 'urine'"
                          :size="12"
                        /><ClipboardList v-else :size="12" />{{
                          r.kind === "urine" ? "尿团" : "大便"
                        }}</span
                      >
                    </td>
                    <td>
                      <strong class="record-grams">{{
                        r.grams !== null ? r.grams + " g" : "—"
                      }}</strong
                      ><small>{{
                        r.kind === "urine"
                          ? [
                              r.diameter_cm ? r.diameter_cm + " cm" : "",
                              loose[r.looseness],
                            ]
                              .filter(Boolean)
                              .join(" · ")
                          : [
                              moisture[r.moisture],
                              shapes[r.shape],
                              sizes[r.size],
                            ]
                              .filter(Boolean)
                              .join(" · ")
                      }}</small>
                    </td>
                    <td>
                      {{ boxName(r.box_id)
                      }}<small
                        >{{ r.litter_name }} · 等待{{
                          r.wait_minutes
                        }}分钟</small
                      >
                    </td>
                    <td class="notes-cell">
                      {{ r.notes || "—"
                      }}<span class="blood-tag" v-if="r.blood || r.straining"
                        >{{ r.blood ? "有血迹" : "" }}
                        {{ r.straining ? "排泄用力" : "" }}</span
                      >
                    </td>
                    <td v-if="canWrite">
                      <div class="row-actions">
                        <button
                          class="icon-btn"
                          @click="openModal('record', r)"
                          :aria-label="'编辑记录' + r.id"
                          :title="r.cat_id ? '编辑记录' : '核对归属 / 编辑'"
                        >
                          <Pencil :size="14" /></button
                        ><button
                          class="icon-btn danger"
                          @click="remove('records', r.id)"
                          aria-label="删除记录"
                        >
                          <Trash2 :size="14" />
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
              <div v-if="!filteredRecords.length" class="empty">
                这个范围还没有记录。{{
                  canWrite ? "点击“记一笔屎尿”开始观察吧。" : ""
                }}
              </div>
            </div>
            <div class="table-footer">
              <span
                >共 {{ filteredRecords.length }} 条{{
                  tab === "dashboard" && filteredRecords.length > 5
                    ? " · 展示最近5条"
                    : ""
                }}</span
              ><span
                ><LockKeyhole
                  v-if="dataset.table.visibility === 'private'"
                  :size="12"
                /><Globe v-else :size="12" />{{
                  dataset.table.visibility === "private"
                    ? "仅授权成员可见"
                    : "公开表格 · 登录可见"
                }}</span
              >
            </div>
          </section>
          <template v-if="tab === 'weights'"
            ><section class="panel chart-panel">
              <div class="panel-heading">
                <h3>体重趋势</h3>
                <span class="pill neutral">近30天 · kg</span>
              </div>
              <WeightChart
                :weights="weights"
                :cats="cats"
                :cat-id="selectedCat"
              />
            </section>
            <section class="panel weight-history">
              <div class="panel-heading">
                <h3>称重记录</h3>
                <span class="subtle">{{ filteredWeights.length }} 次记录</span>
              </div>
              <div class="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>时间</th>
                      <th>猫咪</th>
                      <th>体重</th>
                      <th>备注</th>
                      <th v-if="canWrite">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="w in filteredWeights" :key="w.id">
                      <td>{{ stamp(w.occurred_at) }}</td>
                      <td>{{ catName(w.cat_id) }}</td>
                      <td>
                        <strong>{{ w.kg.toFixed(2) }}</strong> kg
                      </td>
                      <td>{{ w.notes || "—" }}</td>
                      <td v-if="canWrite">
                        <button
                          class="icon-btn danger"
                          @click="remove('weights', w.id)"
                          aria-label="删除体重记录"
                        >
                          <Trash2 :size="15" />
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <div v-if="!filteredWeights.length" class="empty">
                  还没有称重记录。
                </div>
              </div>
            </section></template
          >
          <template v-if="tab === 'cats'"
            ><div class="grid-three">
              <article
                v-for="c in cats"
                :key="c.id"
                class="panel cat-profile"
                :class="{ archived: c.archived }"
              >
                <div class="profile-top">
                  <CatFace :color="c.color" /><button
                    v-if="canWrite"
                    class="icon-btn"
                    aria-label="编辑猫咪"
                    @click="openModal('cat', c)"
                  >
                    <Pencil :size="16" />
                  </button>
                </div>
                <h2>
                  {{ c.name
                  }}<span class="pill neutral" v-if="c.archived">已归档</span>
                </h2>
                <p>
                  {{ c.breed || "未填写品种" }} ·
                  {{
                    c.sex === "male"
                      ? "男孩子"
                      : c.sex === "female"
                        ? "女孩子"
                        : "性别未填写"
                  }}
                </p>
                <dl>
                  <dt>生日</dt>
                  <dd>{{ c.birthday || "未填写" }}</dd>
                  <dt>绝育</dt>
                  <dd>{{ c.neutered ? "已绝育" : "未标记" }}</dd>
                  <dt>最近体重</dt>
                  <dd>{{ latestWeight(c.id)?.kg.toFixed(2) || "—" }} kg</dd>
                  <dt>使用猫砂盆</dt>
                  <dd>
                    {{
                      boxes
                        .filter((b) => b.cat_ids.includes(c.id) && !b.archived)
                        .map((b) => b.name)
                        .join("、") || "尚未配置"
                    }}
                  </dd>
                </dl>
                <p class="profile-note">
                  {{ c.notes || "把它的小习惯也记下来吧。" }}
                </p>
              </article>
            </div>
            <div v-if="!cats.length" class="panel empty">
              先添加一只猫咪，再开始记录。
            </div></template
          >
          <template v-if="tab === 'boxes'"
            ><div class="grid-two">
              <article
                class="panel box-card"
                v-for="b in boxes"
                :key="b.id"
                :class="{ archived: b.archived }"
              >
                <div class="panel-heading">
                  <span class="box-illustration"><Box :size="32" /></span
                  ><button
                    v-if="canWrite"
                    class="icon-btn"
                    aria-label="编辑猫砂盆"
                    @click="openModal('box', b)"
                  >
                    <Pencil :size="17" />
                  </button>
                </div>
                <h3>
                  {{ b.name
                  }}<span class="pill neutral" v-if="b.archived">已归档</span>
                </h3>
                <p>{{ b.location || "未填写位置" }}</p>
                <div class="box-cats">
                  <span v-for="id in b.cat_ids" :key="id"
                    ><CatFace :color="getCat(id)?.color" />{{
                      catName(id)
                    }}</span
                  >
                </div>
                <div class="box-litter">
                  <Leaf :size="17" />{{ litterName(b.litter_id)
                  }}<span class="pill sage">{{
                    b.cat_ids.length > 1 ? "多猫共用" : "单猫使用"
                  }}</span>
                </div>
              </article>
            </div>
            <div v-if="!boxes.length" class="panel empty">
              添加猫砂盆，并设置使用猫与猫砂类型。
            </div>
            <section class="panel litter-panel">
              <div class="panel-heading">
                <div>
                  <h3>猫砂与称量方式</h3>
                  <p>更换猫砂配置会建立新版本，历史记录保留原始快照。</p>
                </div>
                <button
                  v-if="canWrite"
                  class="btn secondary small"
                  @click="openModal('litter')"
                >
                  <Plus :size="15" />添加猫砂
                </button>
              </div>
              <div class="litter-row" v-for="l in litters" :key="l.id">
                <span class="soft-icon"><Leaf :size="20" /></span>
                <div>
                  <strong>{{ l.name }}</strong>
                  <p>
                    {{ materialNames[l.material] }} · 等待
                    {{ l.wait_minutes }} 分钟称量 · v{{ l.version
                    }}<span v-if="l.notes"> · {{ l.notes }}</span>
                  </p>
                </div>
                <button
                  v-if="canWrite"
                  class="icon-btn"
                  @click="openModal('litter', l)"
                  aria-label="编辑猫砂"
                >
                  <Pencil :size="16" />
                </button>
              </div>
              <div v-if="!litters.length" class="empty">
                先添加一种猫砂，再配置猫砂盆。
              </div>
            </section></template
          >
          <template v-if="tab === 'share'"
            ><section class="panel sharing-status">
              <span class="sharing-icon"
                ><Globe
                  v-if="dataset.table.visibility === 'public'"
                  :size="30" /><LockKeyhole v-else :size="30"
              /></span>
              <div>
                <h3>
                  {{
                    dataset.table.visibility === "public"
                      ? "这份手账是公开的"
                      : "这份手账只与家人共享"
                  }}
                </h3>
                <p>
                  {{
                    dataset.table.visibility === "public"
                      ? "其他已登录用户可以浏览，并申请协作；编辑权限由表主授予。"
                      : "仅表主和授权成员可查看、记录。表主可以用账号邀请其他家人。"
                  }}
                </p>
              </div>
              <button
                v-if="isOwner"
                class="btn secondary small"
                @click="go('settings')"
              >
                调整可见性
              </button>
            </section>
            <section
              v-if="dataset.role !== 'public'"
              class="panel members-panel"
            >
              <div class="panel-heading">
                <h3>一起照顾的家人</h3>
                <span class="subtle">权限随时可调整</span>
              </div>
              <form v-if="isOwner" class="invite-form" @submit.prevent="invite">
                <input
                  v-model="inviteName"
                  required
                  placeholder="输入已注册的账号"
                  aria-label="邀请账号"
                /><select v-model="inviteRole" aria-label="邀请权限">
                  <option value="editor">共同管理</option>
                  <option value="viewer">仅浏览</option></select
                ><button class="btn primary small" :disabled="busy">
                  <Plus :size="16" />邀请加入
                </button>
              </form>
              <div class="member-row" v-for="m in members" :key="m.user_id">
                <span class="profile-avatar">{{
                  m.display_name.slice(0, 1)
                }}</span>
                <div class="member-name">
                  <strong>{{ m.display_name }}</strong
                  ><small>@{{ m.username }}</small>
                </div>
                <span
                  class="pill"
                  :class="m.status === 'pending' ? 'amber' : 'neutral'"
                  >{{
                    m.status === "pending"
                      ? "申请中"
                      : m.status === "rejected"
                        ? "已拒绝"
                        : m.status === "removed"
                          ? "已移除"
                          : m.role === "owner"
                            ? "表主"
                            : m.role === "editor"
                              ? "共同管理"
                              : "仅浏览"
                  }}</span
                >
                <div
                  class="member-actions"
                  v-if="isOwner && m.role !== 'owner'"
                >
                  <template v-if="m.status === 'pending'"
                    ><button
                      class="text-btn"
                      :disabled="busy"
                      @click="memberUpdate(m, 'approved', 'editor')"
                    >
                      同意共同管理</button
                    ><button
                      class="text-btn"
                      :disabled="busy"
                      @click="memberUpdate(m, 'approved', 'viewer')"
                    >
                      同意仅浏览</button
                    ><button
                      class="text-btn danger"
                      :disabled="busy"
                      @click="memberUpdate(m, 'rejected')"
                    >
                      拒绝
                    </button></template
                  ><template v-else-if="m.status === 'approved'"
                    ><button
                      class="text-btn"
                      :disabled="busy"
                      @click="
                        memberUpdate(
                          m,
                          'approved',
                          m.role === 'editor' ? 'viewer' : 'editor',
                        )
                      "
                    >
                      {{
                        m.role === "editor" ? "改为仅浏览" : "允许管理"
                      }}</button
                    ><button
                      class="text-btn danger"
                      :disabled="busy"
                      @click="memberUpdate(m, 'removed')"
                    >
                      移除
                    </button></template
                  >
                </div>
              </div>
            </section>
            <div v-else class="panel empty">
              加入协作后可查看成员。
            </div></template
          >
          <template v-if="tab === 'settings'"
            ><form class="panel settings-panel" @submit.prevent="saveSettings">
              <h3>手账的基本设置</h3>
              <label
                >表格名称<input
                  v-model="form.name"
                  required
                  maxlength="80"
                  :disabled="!isOwner" /></label
              ><label
                >一句话介绍<textarea
                  v-model="form.description"
                  maxlength="300"
                  :disabled="!isOwner"
                  rows="3"
                /></label
              ><label>谁可以看见</label>
              <div class="visibility-options">
                <label :class="{ selected: form.visibility === 'private' }"
                  ><input
                    type="radio"
                    v-model="form.visibility"
                    value="private"
                    :disabled="!isOwner"
                  /><LockKeyhole :size="20" /><span
                    >私有<small>仅授权成员可见</small></span
                  ></label
                ><label :class="{ selected: form.visibility === 'public' }"
                  ><input
                    type="radio"
                    v-model="form.visibility"
                    value="public"
                    :disabled="!isOwner"
                  /><Globe :size="20" /><span
                    >公开<small>登录用户可浏览并申请协作</small></span
                  ></label
                >
              </div>
              <p class="form-hint">
                切换为私有后，已授权成员仍可协作，其他用户将无法访问。
              </p>
              <button class="btn primary" v-if="isOwner" :disabled="busy">
                保存设置
              </button>
              <p class="form-hint" v-else>只有表主可以修改设置。</p>
            </form></template
          >
          <footer class="page-footer">
            <PawPrint :size="13" />猫猫手账 · 把在意留在日常里<span>{{
              dataset.table.visibility === "private" ? "私有小家" : "公开手账"
            }}</span>
          </footer>
        </template>
      </div>
    </main>
    <Transition name="toast"
      ><div class="toast" v-if="toast" role="status">
        <Check :size="17" />{{ toast }}
      </div></Transition
    >
  </div>
  <div v-if="modal" class="modal-backdrop" @click.self="!busy && (modal = '')">
    <section
      class="modal"
      role="dialog"
      aria-modal="true"
      :aria-label="
        {
          record: '记录屎尿',
          weight: '记录体重',
          cat: '猫咪档案',
          box: '猫砂盆',
          litter: '猫砂配置',
          table: '新建表格',
        }[modal]
      "
    >
      <div class="modal-heading">
        <div>
          <span class="eyebrow">A LITTLE NOTE OF CARE</span>
          <h2>
            {{ form.id ? "编辑" : "添加"
            }}{{
              {
                record: "一笔屎尿记录",
                weight: "体重记录",
                cat: "猫咪档案",
                box: "猫砂盆",
                litter: "猫砂配置",
                table: "管理表格",
              }[modal]
            }}
          </h2>
        </div>
        <button
          class="icon-btn"
          :disabled="busy"
          @click="modal = ''"
          aria-label="关闭弹窗"
        >
          <X :size="21" />
        </button>
      </div>
      <form @submit.prevent="save" class="modal-form">
        <template v-if="modal === 'table'"
          ><label
            >表格名称<input
              v-model="form.name"
              required
              maxlength="80"
              placeholder="比如：猫猫的生活手账" /></label
          ><label
            >介绍<textarea
              v-model="form.description"
              maxlength="300"
              rows="3"
              placeholder="给这份手账写一句话"
            /></label
          ><label
            >可见性<select v-model="form.visibility">
              <option value="private">私有 · 仅授权成员可见</option>
              <option value="public">公开 · 登录用户可浏览、申请协作</option>
            </select></label
          ></template
        >
        <template v-if="modal === 'cat'"
          ><div class="form-grid">
            <label
              >猫咪名字<input
                v-model="form.name"
                required
                maxlength="40"
                placeholder="它的小名" /></label
            ><label
              >品种<input
                v-model="form.breed"
                maxlength="60"
                placeholder="比如：中华田园猫" /></label
            ><label
              >性别<select v-model="form.sex">
                <option value="unknown">未填写</option>
                <option value="male">男孩子</option>
                <option value="female">女孩子</option>
              </select></label
            ><label>生日<input v-model="form.birthday" type="date" /></label
            ><label
              >头像颜色<select v-model="form.color">
                <option value="sage">鼠尾草绿</option>
                <option value="peach">奶桃色</option>
                <option value="lavender">薰衣草紫</option>
                <option value="blue">雾蓝色</option>
              </select></label
            >
          </div>
          <label class="check-label"
            ><input type="checkbox" v-model="form.neutered" />已经绝育</label
          ><label
            >小习惯与备注<textarea
              v-model="form.notes"
              rows="2"
              maxlength="500"
            /></label
          ><label v-if="form.id" class="check-label"
            ><input
              type="checkbox"
              v-model="form.archived"
            />归档猫咪（保留历史记录）</label
          ></template
        >
        <template v-if="modal === 'litter'"
          ><div class="form-grid">
            <label
              >猫砂名称<input
                v-model="form.name"
                required
                maxlength="60"
                placeholder="品牌 / 型号" /></label
            ><label
              >猫砂类型<select v-model="form.material">
                <option v-for="(label, k) in materialNames" :key="k" :value="k">
                  {{ label }}
                </option>
              </select></label
            >
          </div>
          <label
            >等待多久后称量（分钟）<input
              v-model.number="form.wait_minutes"
              type="number"
              min="0"
              max="1440"
              required /></label
          ><label
            >称量方法<textarea
              v-model="form.notes"
              rows="2"
              maxlength="300"
              placeholder="例如：10分钟后铲出、轻抖3次、同一台秤"
            />
          </label>
          <p class="form-hint">
            {{
              form.id
                ? "保存会生成新版本，新记录重新积累同配置基线。"
                : "猫砂类型和称量方法会影响尿团重量，请尽量保持一致。"
            }}
          </p></template
        >
        <template v-if="modal === 'box'"
          ><div class="form-grid">
            <label
              >猫砂盆名字<input
                v-model="form.name"
                required
                maxlength="60" /></label
            ><label
              >所在位置<input
                v-model="form.location"
                maxlength="80"
                placeholder="客厅 · 窗边"
            /></label>
          </div>
          <label
            >使用猫砂<select v-model.number="form.litter_id" required>
              <option disabled value="">先添加猫砂</option>
              <option v-for="l in litters" :key="l.id" :value="l.id">
                {{ l.name }} · {{ materialNames[l.material] }}
              </option>
            </select></label
          ><button type="button" class="text-btn" @click="openModal('litter')">
            <Plus :size="14" />添加新的猫砂类型</button
          ><label>哪些猫会使用这个盆</label>
          <div class="checkbox-cats">
            <label v-for="c in cats" :key="c.id"
              ><input
                type="checkbox"
                :value="c.id"
                v-model="form.cat_ids"
              /><CatFace :color="c.color" />{{ c.name }}</label
            >
          </div>
          <p class="form-hint" v-if="!activeCats.length">请先添加猫咪。</p>
          <label v-if="form.id" class="check-label"
            ><input
              type="checkbox"
              v-model="form.archived"
            />归档猫砂盆（保留历史）</label
          ></template
        >
        <template v-if="modal === 'weight'"
          ><div class="form-grid">
            <label
              >哪一只猫<select v-model.number="form.cat_id" required>
                <option value="" disabled>选择猫咪</option>
                <option v-for="c in activeCats" :key="c.id" :value="c.id">
                  {{ c.name }}
                </option>
              </select></label
            ><label
              >称重时间<input
                type="datetime-local"
                v-model="form.occurred_at"
                required /></label
            ><label
              >体重（kg）<input
                type="number"
                v-model="form.kg"
                min="0.01"
                max="30"
                step="0.01"
                required
                placeholder="例如：4.32"
            /></label>
          </div>
          <label
            >备注<textarea
              v-model="form.notes"
              rows="2"
              maxlength="300"
              placeholder="称重时间、饭前饭后等"
            /></label
        ></template>
        <template v-if="modal === 'record'"
          ><div class="segmented record-type">
            <button
              type="button"
              :class="{ active: form.kind === 'urine' }"
              @click="form.kind = 'urine'"
            >
              <Droplets :size="17" />尿团</button
            ><button
              type="button"
              :class="{ active: form.kind === 'stool' }"
              @click="form.kind = 'stool'"
            >
              <ClipboardList :size="17" />大便
            </button>
          </div>
          <div class="form-grid">
            <label
              >猫砂盆<select
                v-model.number="form.box_id"
                @change="onBoxChange"
                required
              >
                <option value="" disabled>先配置猫砂盆</option>
                <option
                  v-for="b in boxes.filter(
                    (b) => !b.archived || b.id === form.box_id,
                  )"
                  :key="b.id"
                  :value="b.id"
                >
                  {{ b.name }}
                </option>
              </select></label
            ><label
              >观察时间<input
                type="datetime-local"
                v-model="form.occurred_at"
                required /></label
            ><label class="full"
              >猫咪归属<select v-model="form.cat_id">
                <option value="">不确定 / 混合收集</option>
                <option v-for="c in eligibleCats" :key="c.id" :value="c.id">
                  {{ c.name }}（已目击 / 身份确认）
                </option>
              </select></label
            >
          </div>
          <div class="form-note" v-if="!form.cat_id">
            <CircleHelp :size="16" />
            <p>
              未知归属也可以先记下来。匹配分只提供核对线索，请在目击或身份确认后再选择猫咪。
            </p>
          </div>
          <p class="form-hint" v-if="selectedBox">
            猫砂：{{ form.litter_name || litterName(selectedBox.litter_id) }} ·
            配置等待
            {{
              form.wait_minutes ??
              litters.find((l) => l.id === selectedBox.litter_id)?.wait_minutes
            }}
            分钟后称量
          </p>
          <div class="form-grid">
            <label
              >个数 / 份数<input
                v-model.number="form.count"
                type="number"
                :min="form.no_output ? 0 : 1"
                max="100"
                :disabled="form.no_output"
                required /></label
            ><label
              >总重量（g，可选）<input
                v-model="form.grams"
                type="number"
                min="0.1"
                max="2000"
                step="0.1"
                :disabled="form.no_output"
                placeholder="铲出后称重" /></label
            ><template v-if="form.kind === 'urine' && !form.no_output"
              ><label
                >最大直径（cm，可选）<input
                  v-model="form.diameter_cm"
                  type="number"
                  min="0.1"
                  max="50"
                  step="0.1"
                  placeholder="单团记录可用于对比" /></label
              ><label
                >松散程度<select v-model.number="form.looseness">
                  <option value="">未观察</option>
                  <option v-for="n in 5" :key="n" :value="n">
                    {{ n }} · {{ loose[n] }}
                  </option>
                </select></label
              ></template
            ><template v-if="form.kind === 'stool' && !form.no_output"
              ><label
                >干湿程度<select v-model.number="form.moisture" required>
                  <option v-for="n in 5" :key="n" :value="n">
                    {{ n }} · {{ moisture[n] }}
                  </option>
                </select></label
              ><label
                >形态<select v-model="form.shape" required>
                  <option v-for="(label, k) in shapes" :key="k" :value="k">
                    {{ label }}
                  </option>
                </select></label
              ><label
                >大小<select v-model.number="form.size" required>
                  <option v-for="n in 5" :key="n" :value="n">
                    {{ n }} · {{ sizes[n] }}
                  </option>
                </select></label
              ></template
            ><label
              >现场猫体重（kg，可选）<input
                v-model="form.body_weight_kg"
                type="number"
                min="0.01"
                max="30"
                step="0.01"
                placeholder="仅现场独立称量 / 设备读数"
            /></label>
          </div>
          <div class="symptom-checks">
            <label class="check-label"
              ><input
                v-model="form.no_output"
                type="checkbox"
                @change="onNoOutput"
              />无产出</label
            ><label class="check-label"
              ><input v-model="form.straining" type="checkbox" />明显用力 /
              疼痛</label
            ><label class="check-label"
              ><input v-model="form.blood" type="checkbox" />发现血迹</label
            >
          </div>
          <div
            class="error"
            v-if="form.kind === 'urine' && form.straining && form.no_output"
          >
            排尿用力但无尿：请立即联系兽医，不要等待统计结果。
          </div>
          <label
            >备注<textarea
              v-model="form.notes"
              rows="2"
              maxlength="500"
              placeholder="例如：铲砂时发现，未目击；饮食或猫砂有变化"
            /></label
        ></template>
        <div class="error" v-if="error" role="alert">{{ error }}</div>
        <div class="modal-footer">
          <button
            class="btn secondary"
            type="button"
            @click="modal = ''"
            :disabled="busy"
          >
            取消</button
          ><button class="btn primary" :disabled="busy">
            <LoaderCircle v-if="busy" class="spin" :size="16" /><Check
              v-else
              :size="16"
            />{{ busy ? "保存中…" : "保存记录" }}
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
