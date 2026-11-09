// Builds afe-meter.html (and afe-meter-light.html with --light): a multimeter on the bench,
// black probe on the Pico W's GND pad, red probe on one of the AFE header joints beside the SO2-A4.
// The meter's LCD and the card readout come from one model: what a reading means for the
// 280 mV certificate zero versus the 564/558 mV the Pico publishes.
//   node docs/figures/afe-meter/build.mjs [--light] [--audit]     (ANATOMY=/path/to/anatomy to override)
import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { homedir } from "node:os";

const KIT = process.env.ANATOMY || join(homedir(), ".agents/skills/anatomy");
const k = await import(join(KIT, "kit/iso-kit.mjs"));
const G = await import(join(KIT, "kit/lathe.mjs"));
const T = await import(join(KIT, "kit/tube.mjs"));
const A = await import(join(KIT, "kit/audit.mjs"));
const { kitScript } = await import(join(KIT, "scripts/inline-kit.mjs"));
const HERE = dirname(fileURLToPath(import.meta.url));
const theme = process.argv.includes("--light") ? "light" : "dark";

// ---- The model. Every number on the LCD and in the readout comes from here. ----
const CERT = { we: 280, ae: 281 }; // AFE cert 14-000945, SN1 SO2-A4 electronic offsets (mV)
const PICO = { we: 564, ae: 558 }; // so2mqtt publish, 2026-11-05 17:12 UTC (mV)
const BAND = 45; // ± mV that still counts as "near" either value
const RANGE = 1999; // the 2000m range shows up to 1999; above that the meter shows "1"
const CASES = [0, CERT.we, PICO.we, 2400]; // the tour: ground, cert zero, Pico value, over range

// ---- World, 1 mm = 1.5 units. x and y on the bench, z up. ----
const MM = 1.5;
const PLATE = { x: -298, y: -186, w: 368, d: 330, r: 10 };
const PLATE_H = 10, FOOT_H = 5;

// Maker Pi Pico base (94 x 69 mm) with the Pico W (51 x 21 mm) seated in it, USB end toward +y.
const MAKER = { x: -275, y: -170, w: 69 * MM, d: 94 * MM, r: 3 };
const MAKER_Z = 6, PCB = 1.6;
const MAKER_TOP = MAKER_Z + PCB;
const PICO_SOCK = 8.5 * MM; // female header height
const PICO_W = { x: MAKER.x + (MAKER.w - 21 * MM) / 2, y: MAKER.y + 18, w: 21 * MM, d: 51 * MM, r: 1.2 };
const PICO_Z = MAKER_TOP + PICO_SOCK, PICO_TOP = PICO_Z + 1;
const PITCH = 2.54 * MM;
// Pin 40 is at the USB end on the +x side; pin 38 (GND) is third from that end.
const padAt = (n) => {
  const side = n <= 20 ? 0 : 1;
  const i = side === 0 ? n - 1 : 40 - n; // 0 at the USB end
  return [side === 0 ? PICO_W.x + 1.3 : PICO_W.x + PICO_W.w - 1.3, PICO_W.y + PICO_W.d - 2.2 - i * PITCH];
};
const GND = padAt(38);

// SmartCitizen ASB (42 x 44 mm) and the Alphasense AFE (52 x 28 mm), both on standoffs.
const STAND_H = 12;
const ASB = { x: -130, y: -150, w: 42 * MM, d: 44 * MM, r: 1.5 };
const AFE = { x: -30, y: -150, w: 52 * MM, d: 28 * MM, r: 1.5 };
const BOARD_TOP = STAND_H + PCB;
const AFE_MID_Y = AFE.y + AFE.d / 2;
const SN = [[AFE.x + 13 * MM, AFE_MID_Y], [AFE.x + 39 * MM, AFE_MID_Y]];
const SENSOR_R = 10.1 * MM, SENSOR_H = 16.5 * MM;
const HDR_X = (SN[0][0] + SENSOR_R + SN[1][0] - SENSOR_R) / 2; // header sits between the sensors
const HDR_P = 2 * MM; // 2 mm pitch
const JOINTS = [0, 1].flatMap((col) => [-2, -1, 0, 1, 2].map((row) => [HDR_X + (col - 0.5) * HDR_P, AFE_MID_Y + row * HDR_P]));
const PROBED = 7; // the joint the red probe sits on in the drawing (any of the ten)

// The meter, lying across the front: display at the -x end, jacks at the +x end.
// Its top points -x, so its right-hand side (V jack) points -y, toward the boards.
const METER = { x: -262, y: 18, w: 140 * MM, d: 70 * MM, r: 9 };
const METER_H = 20, BOOT = 3;
const M_TOP = METER_H + 1.4;
const MCY = METER.y + METER.d / 2;
const LCD = { x: METER.x + 14, y: METER.y + 14, w: 50, d: METER.d - 28, r: 2 };
const DIAL = [METER.x + 122, MCY];
const DIAL_R = 22;
const JACK_X = METER.x + METER.w - 18;
const JACKS = { amp: [JACK_X, MCY + 30], com: [JACK_X, MCY], volt: [JACK_X, MCY - 30] };
const PLUG_H = 14, PLUG_R = 4.2;

const P = k.fitProjection(
  [...k.boxCorners(PLATE, -PLATE_H - FOOT_H, 0), [MAKER.x, MAKER.y, 90], [AFE.x + AFE.w, AFE.y, 90]],
  640, 400, { pad: 22, azimuth: 38, elevation: 32 },
);
const V = G.viewOf(P);
const KS = G.scaleOf(P);

const S = (paths, style) => k.solidSvg(paths, style);
const L = (d, style) => k.lineSvg(d, style);
const Dt = (points, style) => k.dotsSvg(points, style);
const at = (p) => k.iso(p, P);
const R = A.recorder(P);

// One drawn item: its svg and the true shapes of what it draws.
function put(name, centre, svg, shapes = [], bias = 0) {
  R.put({ name, at: centre, bias, svg, shapes: shapes.map((s) => R.solid({ ...s, name: s.name ?? name })) });
}
const slab = (name, plan, z, h, tone, opts = {}) => {
  const svg = S(k.slabOf(plan, z, h, P, opts.steps ?? 4, opts.bevel ?? 0), { tone, crease: opts.crease, lit: opts.lit, inner: opts.inner });
  return { svg, shape: A.slab(plan, z, h, { name }, opts.steps ?? 4) };
};
const cyl = (name, x, y, r, z, h, tone, opts = {}) => {
  const svg = S(k.cylinder(x, y, r, z, h, P, opts.steps ?? 24, opts.bevel ?? 0), { tone, crease: opts.crease, lit: opts.lit });
  return { svg, shape: A.cylinder(x, y, r, z, h, { name }) };
};
const mid = (plan, z) => [plan.x + plan.w / 2, plan.y + plan.d / 2, z];

// ---- Bench plate on feet ----
function bench() {
  const out = [`<path class="iso-halo" d="${k.haloOf(PLATE, -PLATE_H - FOOT_H, PLATE_H, P)}"/>`];
  const feet = k.corners(PLATE, 24).sort((a, b) => k.depthOf([...a, 0], P) - k.depthOf([...b, 0], P));
  for (const [x, y] of feet) {
    out.push(S(k.cylinder(x, y, 8, -PLATE_H - FOOT_H, 1.6, P, 28), { tone: "lo" }));
    out.push(S(k.cylinder(x, y, 6, -PLATE_H - FOOT_H + 1.6, FOOT_H - 1.6, P, 28), { tone: "mid" }));
  }
  out.push(S(k.slabOf(PLATE, -PLATE_H, PLATE_H, P, 8, 1.6), { tone: "mid", inner: L(k.sideSeam(PLATE, -PLATE_H / 2, P, 8), { tone: "faint" }) }));
  out.push(L(k.planOutline(k.insetPlan(PLATE, 6), 0, P), { tone: "lo" }));
  const screws = k.corners(PLATE, 12);
  out.push(L(screws.map(([x, y]) => k.ring(x, y, 2.4, 0, P, 16)).join(""), { tone: "lo" }));
  out.push(Dt(screws.map(([x, y]) => at([x, y, 0])), { size: 0.5 }));
  // ruler strip along the front edge, ticks hanging from its edge
  const strip = { x: PLATE.x + 40, y: PLATE.y + PLATE.d - 16, w: PLATE.w - 80, d: 7, r: 1 };
  out.push(S(k.slabOf(strip, 0, 0.6, P, 3), { tone: "lo", crease: "none" }));
  const t = k.topTicks(strip.x + 4, strip.x + strip.w - 4, 1.5 * MM, 10, strip.y + 0.8, 0.6, [2, 3.6], P);
  out.push(L(t.minor, { tone: "lo" }), L(t.major, { tone: "mid" }));
  return out.join("");
}
R.solid(A.slab(PLATE, -PLATE_H, PLATE_H, { name: "stand.plate" }, 8));

// ---- Maker Pi Pico base with the Pico W ----
function makerBase() {
  // rubber bumpers
  for (const [x, y] of k.corners(MAKER, 7)) {
    const c = cyl("maker.bumper", x, y, 4, 0, MAKER_Z, "lo", { steps: 18 });
    put("maker.bumper", [x, y, MAKER_Z / 2], c.svg, [c.shape]);
  }
  const holes = k.corners(MAKER, 7);
  const board = slab("maker.pcb", MAKER, MAKER_Z, PCB, "mid", { steps: 6, bevel: 0.6 });
  const extra = [
    L(k.planOutline(k.insetPlan(MAKER, 4), MAKER_TOP, P, 6), { tone: "faint" }),
    L(holes.map(([x, y]) => k.ring(x, y, 2.6, MAKER_TOP, P, 16)).join(""), { tone: "lo" }),
    Dt(holes.map(([x, y]) => at([x, y, MAKER_TOP])), { size: 0.5 }),
    // three tactile buttons and a row of status LEDs along the -x edge
    L([0, 1, 2].map((i) => k.ring(MAKER.x + 11, MAKER.y + 30 + i * 14, 3.2, MAKER_TOP, P, 16)).join(""), { tone: "lo" }),
    Dt(Array.from({ length: 13 }, (_, i) => at([MAKER.x + 22, MAKER.y + 24 + i * 6.4, MAKER_TOP])), { size: 0.55, tone: "mid" }),
    // the one engraved label: GND beside pin 38
    k.faceTextSvg(k.topMatrix([GND[0] + 13, GND[1] + 0.8], MAKER_TOP, P, "x"), "GND", { size: 3.4, tone: "mid", anchor: "middle" }),
  ];
  put("maker.pcb", mid(MAKER, MAKER_Z), board.svg + extra.join(""), [board.shape]);

  // two female header strips the Pico sits in
  for (const side of [0, 1]) {
    const strip = { x: side ? PICO_W.x + PICO_W.w - 2.6 * MM : PICO_W.x, y: PICO_W.y, w: 2.6 * MM, d: PICO_W.d, r: 0.6 };
    const s = slab("maker.socket", strip, MAKER_TOP, PICO_SOCK, "lo", { steps: 2 });
    const pins = Array.from({ length: 20 }, (_, i) => at([strip.x + strip.w / 2, strip.y + 2.2 + i * PITCH, MAKER_TOP + PICO_SOCK]));
    put("maker.socket", mid(strip, MAKER_TOP), s.svg + Dt(pins, { size: 0.45, tone: "faint" }), [s.shape]);
  }
  // the Pico W module: board, RP2040, shield can, castellated pads
  const pico = slab("pico.pcb", PICO_W, PICO_Z, 1, "mid", { steps: 2, bevel: 0.4 });
  const can = { x: PICO_W.x + 6, y: PICO_W.y + 6, w: 12 * MM, d: 14 * MM, r: 0.8 };
  const chip = { x: PICO_W.x + PICO_W.w / 2 - 5.2, y: PICO_W.y + PICO_W.d / 2 - 2, w: 10.5, d: 10.5, r: 0.4 };
  const pads = Array.from({ length: 40 }, (_, i) => padAt(i + 1)).filter((p) => p !== GND && !(p[0] === GND[0] && p[1] === GND[1]));
  const padRings = [...pads, GND].map(([x, y]) => k.ring(x, y, 1, PICO_TOP, P, 10)).join("");
  put("pico.pcb", mid(PICO_W, PICO_Z), pico.svg + L(padRings, { tone: "lo" }) + Dt(pads.map(([x, y]) => at([x, y, PICO_TOP])), { size: 0.35, tone: "faint" }), [pico.shape]);
  const c = slab("pico.can", can, PICO_TOP, 3, "hi", { steps: 2, bevel: 0.5 });
  put("pico.can", mid(can, PICO_TOP), c.svg + Dt([at([can.x + 2, can.y + 2, PICO_TOP + 3])], { size: 0.5 }), [c.shape]);
  const r = slab("pico.rp2040", chip, PICO_TOP, 1.2, "lo", { steps: 2 });
  put("pico.rp2040", mid(chip, PICO_TOP), r.svg + Dt([at([chip.x + 1.6, chip.y + 1.6, PICO_TOP + 1.2])], { size: 0.45, tone: "mid" }), [r.shape]);
  const usb = { x: PICO_W.x + PICO_W.w / 2 - 6, y: PICO_W.y + PICO_W.d - 7, w: 12, d: 8.5, r: 1 };
  const u = slab("pico.usb", usb, PICO_TOP, 4, "mid", { steps: 2, bevel: 0.4 });
  put("pico.usb", mid(usb, PICO_TOP), u.svg, [u.shape]);
  // the GND pad the black probe touches: a raised solder pad
  const g = cyl("pico.gnd", GND[0], GND[1], 1.15, PICO_TOP, 0.5, "hi", { steps: 14 });
  put("pico.gnd", [GND[0], GND[1], PICO_TOP], g.svg, [g.shape]);

  // Grove socket on the +x edge (Grove 5), to the ASB
  const grove = { x: MAKER.x + MAKER.w - 13, y: ASB.y + 26, w: 12, d: 15, r: 0.8 };
  const gs = slab("maker.grove", grove, MAKER_TOP, 7.5, "lo", { steps: 2, bevel: 0.4 });
  put("maker.grove", mid(grove, MAKER_TOP), gs.svg + L(k.planOutline(k.insetPlan(grove, 1.8), MAKER_TOP + 7.5, P, 2), { tone: "faint" }), [gs.shape]);
  return { grove };
}

// ---- A board on four standoffs, with corner screws ----
function standoffs(prefix, plan) {
  const posts = k.corners(plan, 3.5);
  for (const [x, y] of posts) {
    const s = cyl(`${prefix}.standoff`, x, y, 2.6, 0, STAND_H, "mid", { steps: 16, crease: "none" });
    put(`${prefix}.standoff`, [x, y, STAND_H / 2], s.svg, [s.shape]);
  }
  return posts;
}
function screws(posts) {
  return L(posts.map(([x, y]) => k.ring(x, y, 2.4, BOARD_TOP, P, 14)).join(""), { tone: "lo" })
    + L(posts.map(([x, y]) => k.lineOnTop([x - 1.5, y - 1.5], [x + 1.5, y + 1.5], BOARD_TOP, P)).join(""), { tone: "mid" });
}

// ---- SmartCitizen analog sensor board ----
function asb() {
  const posts = standoffs("asb", ASB);
  const b = slab("asb.pcb", ASB, STAND_H, PCB, "mid", { steps: 4, bevel: 0.5 });
  const traces = [0, 1, 2, 3].map((i) => k.lineOnTop([ASB.x + 16, ASB.y + 22 + i * 3], [ASB.x + ASB.w - 14, ASB.y + 22 + i * 3], BOARD_TOP, P)).join("");
  const vias = [0, 1, 2, 3].flatMap((i) => [at([ASB.x + 16, ASB.y + 22 + i * 3, BOARD_TOP]), at([ASB.x + ASB.w - 14, ASB.y + 22 + i * 3, BOARD_TOP])]);
  put("asb.pcb", mid(ASB, STAND_H), b.svg + screws(posts) + L(traces, { tone: "faint" }) + Dt(vias, { size: 0.6, tone: "lo" }), [b.shape]);
  // two ADS1115 (0x48, 0x49), a boost inductor, the level shifter
  [0, 1].forEach((i) => {
    const chip = { x: ASB.x + 22 + i * 22, y: ASB.y + 38, w: 9, d: 9, r: 0.3 };
    const c = slab("asb.ads", chip, BOARD_TOP, 1.4, "lo", { steps: 2 });
    const legs = [0, 1, 2, 3, 4].flatMap((j) => [at([chip.x + 1.2 + j * 1.65, chip.y - 0.6, BOARD_TOP]), at([chip.x + 1.2 + j * 1.65, chip.y + chip.d + 0.6, BOARD_TOP])]);
    put("asb.ads", mid(chip, BOARD_TOP), c.svg + Dt(legs, { size: 0.3, tone: "mid" }) + Dt([at([chip.x + 1.4, chip.y + 1.4, BOARD_TOP + 1.4])], { size: 0.35, tone: "mid" }), [c.shape]);
  });
  const ind = cyl("asb.inductor", ASB.x + 24, ASB.y + 12, 4.5, BOARD_TOP, 4, "lo", { steps: 20, bevel: 0.4 });
  put("asb.inductor", [ASB.x + 24, ASB.y + 12, BOARD_TOP], ind.svg + L(k.ring(ASB.x + 24, ASB.y + 12, 2, BOARD_TOP + 4, P, 12), { tone: "faint" }), [ind.shape]);
  const ls = { x: ASB.x + 44, y: ASB.y + 9, w: 8, d: 6, r: 0.3 };
  const l2 = slab("asb.shifter", ls, BOARD_TOP, 1.2, "lo", { steps: 2 });
  put("asb.shifter", mid(ls, BOARD_TOP), l2.svg, [l2.shape]);
  // Grove socket J2 on the -x edge
  const grove = { x: ASB.x + 2, y: ASB.y + 26, w: 12, d: 15, r: 0.8 };
  const gs = slab("asb.grove", grove, BOARD_TOP, 7.5, "lo", { steps: 2, bevel: 0.4 });
  put("asb.grove", mid(grove, BOARD_TOP), gs.svg + L(k.planOutline(k.insetPlan(grove, 1.8), BOARD_TOP + 7.5, P, 2), { tone: "faint" }), [gs.shape]);
  // J5 IDC plug hangs under the board on the +x edge; the ribbon leaves it toward the AFE
  const idc = { x: ASB.x + ASB.w - 14, y: AFE_MID_Y - 8, w: 11, d: 16, r: 0.6 };
  const p = slab("asb.idc", idc, STAND_H - 7, 7, "lo", { steps: 2 });
  put("asb.idc", mid(idc, STAND_H - 7), p.svg, [p.shape]);
  return { grove, idc };
}

// ---- Alphasense AFE, sensor side up ----
function afe(asbIdc) {
  const posts = standoffs("afe", AFE);
  const b = slab("afe.pcb", AFE, STAND_H, PCB, "mid", { steps: 4, bevel: 0.5 });
  put("afe.pcb", mid(AFE, STAND_H), b.svg + screws(posts) + L(k.planOutline(k.insetPlan(AFE, 3), BOARD_TOP, P, 4), { tone: "faint" }), [b.shape]);
  // IDC plug under the board, below the header
  const idc = { x: HDR_X - 6, y: AFE_MID_Y - 8, w: 12, d: 16, r: 0.6 };
  const p = slab("afe.idc", idc, STAND_H - 7, 7, "lo", { steps: 2 });
  put("afe.idc", mid(idc, STAND_H - 7), p.svg, [p.shape]);
  // the ribbon, taut between the two plugs, ten conductors as ridges
  const rib = { x: asbIdc.x + asbIdc.w, y: AFE_MID_Y - 7, w: idc.x - asbIdc.x - asbIdc.w, d: 14, r: 0 };
  const rs = slab("ribbon", rib, STAND_H - 4.6, 0.8, "faint", { steps: 1 });
  const ridges = Array.from({ length: 9 }, (_, i) => k.lineOnTop([rib.x, rib.y + 1.4 * (i + 1)], [rib.x + rib.w, rib.y + 1.4 * (i + 1)], STAND_H - 3.8, P)).join("");
  put("ribbon", mid(rib, STAND_H - 4.6), rs.svg + L(ridges, { tone: "faint" }) + L(k.lineOnTop([rib.x, rib.y + 0.7], [rib.x + rib.w, rib.y + 0.7], STAND_H - 3.8, P), { tone: "red" }), [rs.shape]);

  // SN1: the SO2-A4 in its socket
  const [sx, sy] = SN[0];
  const sock = cyl("afe.sn1.socket", sx, sy, SENSOR_R + 1.2, BOARD_TOP, 2, "lo", { steps: 40 });
  put("afe.sn1.socket", [sx, sy, BOARD_TOP], sock.svg, [sock.shape]);
  const bodyTop = BOARD_TOP + 2 + SENSOR_H;
  const body = S(k.cylinder(sx, sy, SENSOR_R, BOARD_TOP + 2, SENSOR_H - 2.4, P, 56, 0.6), { tone: "hi", crease: "lo", inner: L(k.sideArc(sx, sy, SENSOR_R, BOARD_TOP + 2 + SENSOR_H * 0.42, P, 40), { tone: "faint" }) });
  const cap = S(k.cylinder(sx, sy, SENSOR_R - 2, bodyTop - 2.4, 2.4, P, 48, 0.4), { tone: "hi", crease: "lo" });
  const filter = L(k.ring(sx, sy, 4.5, bodyTop, P, 28), { tone: "lo" }) + L(k.ring(sx, sy, 2.4, bodyTop, P, 20), { tone: "faint" });
  put("afe.sn1.sensor", [sx, sy, BOARD_TOP + 2 + SENSOR_H / 2], body + cap + filter, [
    A.cylinder(sx, sy, SENSOR_R, BOARD_TOP + 2, SENSOR_H - 2.4, {}),
    A.cylinder(sx, sy, SENSOR_R - 2, bodyTop - 2.4, 2.4, { name: "afe.sn1.cap" }),
  ]);

  // SN2: the empty socket, four receptacles in a ring
  const [ex, ey] = SN[1];
  const sock2 = cyl("afe.sn2.socket", ex, ey, SENSOR_R + 1.2, BOARD_TOP, 2, "lo", { steps: 40 });
  const holes = [0, 1, 2, 3].map((i) => [ex + Math.cos(Math.PI / 4 + (i * Math.PI) / 2) * 7, ey + Math.sin(Math.PI / 4 + (i * Math.PI) / 2) * 7]);
  put("afe.sn2.socket", [ex, ey, BOARD_TOP], sock2.svg
    + L(k.ring(ex, ey, SENSOR_R - 1, BOARD_TOP + 2, P, 48), { tone: "faint" })
    + L(holes.map(([x, y]) => k.ring(x, y, 1.5, BOARD_TOP + 2, P, 12)).join(""), { tone: "lo" })
    + Dt(holes.map(([x, y]) => at([x, y, BOARD_TOP + 2])), { size: 0.45, tone: "lo" }), [sock2.shape]);

  // the 2x5 header's solder joints, each a little cone of solder on its pad
  JOINTS.forEach(([x, y], i) => {
    const pad = cyl("afe.joint", x, y, 1.1, BOARD_TOP, 0.9, i === PROBED ? "hi" : "mid", { steps: 12 });
    put(`afe.joint#${i}`, [x, y, BOARD_TOP], pad.svg + L(k.ring(x, y, 1.45, BOARD_TOP, P, 12), { tone: "faint" }), [pad.shape]);
  });
}

// ---- The meter ----
function meter() {
  const boot = { ...METER };
  const out = [`<path class="iso-halo" d="${k.haloOf(METER, 0, METER_H, P)}"/>`];
  const b = slab("meter.boot", boot, 0, METER_H, "lo", { steps: 9, bevel: 1.2, inner: L(k.sideSeam(boot, METER_H * 0.45, P, 9), { tone: "faint" }) });
  const face = { x: METER.x + BOOT, y: METER.y + BOOT, w: METER.w - 2 * BOOT, d: METER.d - 2 * BOOT, r: 7 };
  const f = slab("meter.face", face, METER_H, 1.4, "mid", { steps: 8, bevel: 0.6 });
  // LCD window: recess outline, a lens, digits drawn live by the page
  const lcd = L(k.planOutline(LCD, M_TOP, P, 3), { tone: "lo" }) + L(k.planOutline(k.insetPlan(LCD, 2.2), M_TOP, P, 3), { tone: "faint" });
  // rotary selector: knurled knob, pointer at the DC-volt 2000m position, range ticks round it
  const [dx, dy] = DIAL;
  const ticks = k.radialTicks(dx, dy, DIAL_R + 9, 20, 5, M_TOP, [1.6, 3.2], P);
  // two-button row below the LCD
  const buttons = [-1, 1].map((s) => k.planOutline({ x: LCD.x + LCD.w + 8, y: MCY + s * 16 - 7, w: 6, d: 14, r: 2.5 }, M_TOP, P, 3)).join("");
  out.push(b.svg);
  put("meter.boot", mid(boot, 0), out.join(""), [b.shape]);
  put("meter.face", mid(face, METER_H), f.svg + lcd + L(k.ring(dx, dy, DIAL_R + 9, M_TOP, P, 72), { tone: "faint" }) + L(ticks.minor, { tone: "lo" }) + L(ticks.major, { tone: "mid" }) + L(buttons, { tone: "lo" })
    + `<g data-part="lcd">${lcdText("----")}</g>`, [f.shape]);
  const knob = S(k.cylinder(dx, dy, DIAL_R, M_TOP, 6, P, 48, 0.6), { tone: "mid" });
  const knurl = L(k.knurl(dx, dy, DIAL_R, M_TOP + 0.6, M_TOP + 6, 44, P), { tone: "lo" });
  const dir = [-0.74, 0.67]; // pointer: the meter's upper left, the DCV 2000m detent
  const ptr = k.lineOnTop([dx + dir[0] * 4, dy + dir[1] * 4], [dx + dir[0] * (DIAL_R - 2.5), dy + dir[1] * (DIAL_R - 2.5)], M_TOP + 6, P);
  put("meter.knob", [dx, dy, M_TOP + 3], knob + knurl + L(ptr, { tone: "hi", free: true }) + L(k.ring(dx, dy, 2, M_TOP + 6, P, 14), { tone: "mid" }), [A.cylinder(dx, dy, DIAL_R, M_TOP, 6, { name: "meter.knob" })]);
  // three jacks: 10A, COM, VΩmA
  for (const [key, [x, y]] of Object.entries(JACKS)) {
    const j = cyl(`meter.jack.${key}`, x, y, 6.2, M_TOP, 1.6, "lo", { steps: 28, bevel: 0.4 });
    const bore = L(k.ring(x, y, 3.2, M_TOP + 1.6, P, 20), { tone: "lo" });
    put(`meter.jack.${key}`, [x, y, M_TOP], j.svg + (key === "amp" ? bore + L(k.ring(x, y, 2.2, M_TOP + 1.6, P, 16), { tone: "faint" }) : ""), [j.shape]);
  }
}

// The LCD digits, laid flat on the face in its own plane. Drawn again live.
const LCD_MATRIX = k.topMatrix([LCD.x + LCD.w / 2 + 11, MCY], M_TOP, P, "y");
function lcdText(text) {
  return k.faceTextSvg(LCD_MATRIX, text, { size: 32, tone: "hi", anchor: "middle" }) + k.faceTextSvg(k.topMatrix([LCD.x + LCD.w - 6, LCD.y + 9], M_TOP, P, "y"), "mV", { size: 4, tone: "mid", anchor: "middle" });
}

// ---- Banana plugs, probes and leads ----
const up = [0, 0, 1];
function plug(name, [x, y], lit) {
  const z0 = M_TOP + 1.6;
  const body = S(k.cylinder(x, y, PLUG_R, z0, PLUG_H, P, 28, 0.4), { tone: lit ? "hi" : "lo", inner: L(k.sideArc(x, y, PLUG_R, z0 + 4, P, 20), { tone: "faint" }) });
  const boot2 = S(k.cylinder(x, y, PLUG_R - 1.4, z0 + PLUG_H, 4, P, 20), { tone: lit ? "hi" : "lo" });
  put(name, [x, y, z0 + PLUG_H / 2], body + boot2, [A.cylinder(x, y, PLUG_R, z0, PLUG_H, {}), A.cylinder(x, y, PLUG_R - 1.4, z0 + PLUG_H, 4, { name: `${name}.boot` })]);
  return [x, y, z0 + PLUG_H + 4];
}
// A probe: needle, shroud, finger guard, ribbed handle, strain relief. Tip at `tip`, axis toward `dir`.
const PROBE = [[0, 0], [0.8, 0.55], [14, 0.55], [14, 1.6], [18, 3.2], [22, 3.2], [22, 7], [25, 7], [25, 4.4], [70, 4.7], [70, 3.4], [80, 2.4], [80, 0]];
const PROBE_LEN = 80;
function probe(name, tip, dir, lit) {
  const F = G.frameAlong(tip, G.unit3(dir));
  const paths = G.solidOf(PROBE, F, P);
  const ribs = G.ribsOf(F, 32, 62, 4.6, 16, P, { fade: [0.1, 0.55], seams: true });
  const svg = `<g class="probe probe-${lit ? "red" : "black"}">${S(paths, { tone: lit ? "hi" : "lo", lit, inner: k.fadedSvg(ribs, { tone: lit ? "lo" : "faint" }) })}</g>`;
  put(name, G.pointOf(F, 40, 0, 0), svg, [A.solid(F, PROBE, {})]);
  return { tail: G.pointOf(F, PROBE_LEN, 0, 0), axis: F.a };
}
const LEAD_R = 1.5;
function lead(name, from, to, toAxis, via, tone) {
  const pts = [from, G.add3(from, [0, 0, 22]), ...via, G.add3(to, G.mul3(toAxis, 18)), to];
  const route = G.fillet(pts, 10 * LEAD_R, 9, { P, tube: LEAD_R });
  const pieces = T.tubePieces(route, LEAD_R, P, { maxLength: 14 });
  pieces.forEach((piece, i) => R.put({ name: `${name}#${i}`, at: piece.mid, svg: `<g class="lead lead-${tone}">${T.tubeSvg(piece, { tone: "hi" })}</g>`, route: name, chunk: i, shapes: [A.tube(piece.points, piece.radii)] }));
  R.route(name, route, LEAD_R, { limp: true });
}

// ---- Assemble ----
const benchSvg = bench();
const { grove: makerGrove } = makerBase();
const { grove: asbGrove, idc: asbIdc } = asb();
afe(asbIdc);
meter();

// Grove cable: Maker Grove 5 to ASB J2, leaving each socket's top face
// side-entry Grove sockets: the cable leaves the Maker socket's +x face and enters the ASB socket's -x face
const groveA = [makerGrove.x + makerGrove.w, makerGrove.y + makerGrove.d / 2, MAKER_TOP + 3.75];
const groveB = [asbGrove.x, asbGrove.y + asbGrove.d / 2, BOARD_TOP + 3.75];
{
  const route = G.fillet([groveA, G.add3(groveA, [8, 0, 0]), G.add3(groveB, [-8, 0, 0]), groveB], 9, 9, { P, tube: 2 });
  T.tubePieces(route, 2, P, { maxLength: 14 }).forEach((piece, i) => R.put({ name: `grove#${i}`, at: piece.mid, svg: T.tubeSvg(piece, { tone: "mid" }), route: "grove", chunk: i, shapes: [A.tube(piece.points, piece.radii)] }));
  R.route("grove", route, 2, { limp: true });
}

const blackTop = plug("meter.plug.com", JACKS.com, false);
const redTop = plug("meter.plug.volt", JACKS.volt, true);
const black = probe("probe.black", [GND[0], GND[1], PICO_TOP + 0.5], [-0.62, 0.55, 0.56], false);
const joint = JOINTS[PROBED];
const red = probe("probe.red", [joint[0], joint[1], BOARD_TOP + 0.9], [0.25, 0.85, 0.46], true);
lead("lead.black", blackTop, black.tail, black.axis, [[JACK_X + 8, METER.y + METER.d + 10, blackTop[2] + 4], [JACK_X + 8, METER.y + METER.d + 12, LEAD_R * 1.6 + 12], [JACK_X - 20, METER.y + METER.d + 13, LEAD_R * 1.6], [PLATE.x + 16, METER.y + METER.d + 13, LEAD_R * 1.6], [PLATE.x + 16, black.tail[1] + 24, LEAD_R * 1.6]], "black");
lead("lead.red", redTop, red.tail, red.axis, [[JACK_X - 10, MCY - 50, redTop[2] + 14]], "red");

A.settle(R, P);
R.items.sort((a, b) => a.key - b.key);
A.auditOrExit(R, P, { crowd: [] });

const svg = k.figureSvg({
  width: 640,
  height: 400,
  label: "A bench with a Pico W in its Maker base, a Grove cable to the SmartCitizen analog sensor board, a ribbon to the Alphasense AFE with the SO2-A4 sensor in its first socket, and a handheld multimeter. The black lead runs from COM to the Pico's GND pad, the red lead from the V jack to one of the AFE header's ten solder joints. The meter's display shows a reading; the card says what that reading means.",
  body: [benchSvg, ...R.items.map((item) => item.svg)],
});

const live = `
const CERT = ${JSON.stringify(CERT)}, PICO = ${JSON.stringify(PICO)}, BAND = ${BAND}, RANGE = ${RANGE};
const CASES = ${JSON.stringify(CASES)};
const LCD_MATRIX = ${JSON.stringify(LCD_MATRIX)};
const stage = document.querySelector(".iso-stage");
const lcd = stage.querySelector('[data-part="lcd"] text');
const readout = document.querySelector("[data-readout]");
const still = matchMedia("(prefers-reduced-motion: reduce)");
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
function meaning(mv) {
  if (mv > RANGE) return "over range · use 20 V: supply or ref";
  if (mv < 25) return mv.toFixed(0) + " mV · ground pin";
  if (Math.abs(mv - CERT.we) <= BAND) return mv.toFixed(0) + " mV · cert zero: ASB doubles";
  if (Math.abs(mv - PICO.we) <= BAND) return mv.toFixed(0) + " mV · Pico value: AFE high";
  return mv.toFixed(0) + " mV · other pin: note it";
}
const show = (mv) => (mv > RANGE ? "1   " : String(Math.round(mv)).padStart(4, " "));
let value = CASES[0], target = value, frame = 0, last = 0, clock = 0, touring = !still.matches, visible = false, idle = 0;
function draw() {
  const text = show(value);
  if (lcd.textContent !== text) lcd.textContent = text;
  const say = meaning(value);
  if (readout.textContent !== say) readout.textContent = say;
  stage.setAttribute("aria-valuenow", String(Math.round(Math.min(value, 2000))));
  stage.setAttribute("aria-valuetext", say);
}
function tick(now) {
  const dt = last ? Math.min((now - last) / 1000, 1 / 30) : 1 / 60;
  last = now;
  if (touring) { clock += dt; target = CASES[Math.floor(clock / 2.8) % CASES.length]; }
  value = still.matches ? target : value + (target - value) * (1 - Math.exp(-dt / 0.22));
  if (Math.abs(target - value) < 0.5) value = target;
  draw();
  frame = visible && (touring || value !== target) ? requestAnimationFrame(tick) : 0;
}
const run = () => { if (!frame && visible) { last = 0; frame = requestAnimationFrame(tick); } };
const steer = (mv) => { touring = false; target = clamp(mv, 0, 2400); clearTimeout(idle); idle = setTimeout(() => { touring = !still.matches; clock = 0; run(); }, 4000); run(); };
new IntersectionObserver((e) => { visible = e[e.length - 1].isIntersecting; if (visible) run(); }, { rootMargin: "120px 0px" }).observe(stage);
stage.addEventListener("pointermove", (event) => {
  const box = stage.getBoundingClientRect();
  steer(clamp((event.clientX - box.left) / box.width, 0, 1) * 2400);
});
stage.addEventListener("keydown", (event) => {
  const here = CASES.reduce((best, c, i) => (Math.abs(c - target) < Math.abs(CASES[best] - target) ? i : best), 0);
  let to = null;
  if (event.key === "ArrowRight" || event.key === "ArrowUp") to = Math.min(CASES.length - 1, here + 1);
  if (event.key === "ArrowLeft" || event.key === "ArrowDown") to = Math.max(0, here - 1);
  if (to === null) return;
  event.preventDefault();
  steer(CASES[to]);
});
draw();
`;

const css = `<style>
.iso .lead-red{--lead:#d43c32}.iso .lead-black{--lead:#2a2a2a}
.iso .lead .tb-body{fill:var(--lead)}
.iso .probe-red :is(.iso-top,.iso-fill){fill:#c8372e}.iso .probe-red .iso-shade{fill:#a52c25}
.iso .probe-black :is(.iso-top,.iso-fill,.iso-shade){fill:#262626}
.iso[data-theme="light"] .lead-black{--lead:#3a3a3a}
.iso .lead-red .tb-shade{fill:#8c1f19;opacity:.55}.iso .lead-red .tb-shine{fill:#ff8a80;opacity:.5}
.iso [data-part="lcd"] text{font-family:"DSEG7 Classic","Courier New",monospace;font-weight:700;letter-spacing:.06em}
</style>`;

const body = css + k.plateHtml({
  fig: "Fig 1",
  title: "Finding the AFE's zero",
  hint: "Hover to try a reading",
  readout: meaning0(CASES[0]),
  keys: [
    { mark: "lit", label: "Red lead: V jack to one of the ten AFE header joints" },
    { mark: "raised", label: "Black lead: COM jack to the Pico's pin 38 (GND)" },
    { mark: "edge", label: "Dial on DC volts, 2000m range: the display reads millivolts" },
  ],
  caption:
    `Meter set to DC volts on the 2000m range, red lead in the V jack (never 10A), black in COM. Hold the black probe on a ground pad and touch the red probe to each of the AFE header's ten joints in turn, writing each number down. A pin near ${CERT.we} mV matches the certificate, so the analog board doubles the signal on its way to the ADC; a pin near ${PICO.we} mV matches what the Pico publishes, so the AFE itself sits high. We don't know which joint is which yet, so the drawing's probe could be on any of them.`,
  body: `<div class="iso-stage" tabindex="0" role="slider" aria-label="Meter reading: arrow keys step through ground, the certificate zero, the Pico's value and over range" aria-valuemin="0" aria-valuemax="2000" aria-valuenow="0">${svg}</div>`,
});
function meaning0(mv) { return mv.toFixed(0) + " mV · ground pin"; }

const file = theme === "light" ? "afe-meter-light.html" : "afe-meter.html";
writeFileSync(join(HERE, file), k.pageHtml({ title: "Finding the AFE zero", theme, body, script: kitScript() + live }));
console.log("wrote", file, "·", R.items.length, "items");
