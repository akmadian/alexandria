/* <pharos-ascii> — the Pharos of Alexandria, raymarched as SDFs and post-processed
   in a single WebGL fragment shader. Self-contained web component (no deps).

   The 3D scene (signed distance functions + lighting) is fully decoupled from the
   final "look": u_mode picks how the per-cell luminance is drawn —
     0 ascii   luminance -> glyph from a font-atlas texture
     1 dither  ordered Bayer threshold (1-bit stipple)
     2 shaded  plain luminance (a normal render)

   Geometry follows the historical reconstructions: a large sprawling walled
   compound with blocky corner bastions, a slender tapering SQUARE first tier
   (lit windows at night), an octagonal middle tier, an OPEN colonnade cupola
   (thin pillars on a plinth, fire within, conical roof) and a human statue
   reaching to the sky. Optional sweeping light beam.

   Config via attributes OR JS properties (both work). All optional:
     cell        ASCII cell height px — smaller = higher resolution (default 10)
     mode        0 ascii | 1 dither | 2 shaded (default 0)
     grain       dither/film grain 0..0.6 (default 0.15)
     beam        "1"/"0" rotating beam (default 1)
     beam-speed  rad/sec (default 0.5)
     orbit-speed auto-orbit rad/sec (default 0.2)
     orbit-dir   "1" | "-1" (default 1)
     elevation   camera pitch rad (default 0.15)
     height      vertical framing offset (default 0)
     drift       slow vertical bob amount (default 0)
     zoom        camera distance (default 12)
     ramp        glyph-ramp index 0..2 (default 0)
     auto        "1"/"0" auto-orbit (default 1)
     interactive present = drag to orbit / scroll to zoom
   Emits "fps" (detail:number), and while interactive "autochange"/"orbitchange"/"zoomchange". */
(() => {
  if (customElements.get("pharos-ascii")) return;

  const VERT = `attribute vec2 a_pos; void main(){ gl_Position = vec4(a_pos,0.0,1.0); }`;

  const FRAG_SCENE = `
  precision highp float;
  uniform vec2  u_res;
  uniform float u_time, u_yaw, u_pitch, u_zoom, u_camH;
  uniform vec2  u_cell;
  uniform float u_nchars, u_grain, u_mode;
  uniform float u_beam, u_beamSpeed, u_beamDir;
  uniform float u_lightAz, u_lightEl, u_shadow;
  uniform sampler2D u_atlas;

  float sdBox(vec3 p, vec3 b){ vec3 q=abs(p)-b; return length(max(q,0.0))+min(max(q.x,max(q.y,q.z)),0.0); }
  float sdCyl(vec3 p, float h, float r){ vec2 d=abs(vec2(length(p.xz),p.y))-vec2(r,h); return min(max(d.x,d.y),0.0)+length(max(d,0.0)); }
  float sdCone(vec3 p, float h, float ra, float rb){
    vec2 q=vec2(length(p.xz), p.y);
    vec2 k1=vec2(rb,h); vec2 k2=vec2(rb-ra,2.0*h);
    vec2 ca=vec2(q.x-min(q.x,(q.y<0.0)?ra:rb), abs(q.y)-h);
    vec2 cb=q-k1+k2*clamp(dot(k1-q,k2)/dot(k2,k2),0.0,1.0);
    float s=(cb.x<0.0 && ca.y<0.0)?-1.0:1.0;
    return s*sqrt(min(dot(ca,ca),dot(cb,cb)));
  }
  float sdEllipsoid(vec3 p, vec3 r){ float k0=length(p/r); float k1=length(p/(r*r)); return k0*(k0-1.0)/k1; }
  float sdCapsule(vec3 p, vec3 a, vec3 b, float r){
    vec3 pa=p-a, ba=b-a; float h=clamp(dot(pa,ba)/dot(ba,ba),0.0,1.0);
    return length(pa-ba*h)-r;
  }
  float taperSquare(vec3 p, float ymin, float ymax, float rb, float rt){
    float H=ymax-ymin; float ty=clamp((p.y-ymin)/H,0.0,1.0); float s=mix(rb,rt,ty);
    vec3 q=vec3(p.x, p.y-0.5*(ymin+ymax), p.z);
    float cx=max(abs(q.x),abs(q.z));
    vec2 d=vec2(cx-s, abs(q.y)-0.5*H);
    return min(max(d.x,d.y),0.0)+length(max(d,0.0));
  }
  float taperOct(vec3 p, float ymin, float ymax, float rb, float rt){
    float H=ymax-ymin; float ty=clamp((p.y-ymin)/H,0.0,1.0); float s=mix(rb,rt,ty);
    vec3 q=vec3(p.x, p.y-0.5*(ymin+ymax), p.z);
    float cx=max(max(abs(q.x),abs(q.z)), (abs(q.x)+abs(q.z))*0.7071);
    vec2 d=vec2(cx-s, abs(q.y)-0.5*H);
    return min(max(d.x,d.y),0.0)+length(max(d,0.0));
  }
  // ring of thin columns (open colonnade) via angular domain folding
  float colonnade(vec3 p, float ymin, float ymax, float ring, float colR, float n){
    float ang=atan(p.z, p.x);
    float sec=6.2831853/n;
    float k=mod(ang+0.5*sec, sec)-0.5*sec;
    float rad=length(p.xz);
    vec3 fp=vec3(rad*cos(k), p.y-0.5*(ymin+ymax), rad*sin(k));
    return sdCyl(vec3(fp.x-ring, fp.y, fp.z), 0.5*(ymax-ymin), colR);
  }
  vec2 opU(vec2 a, vec2 b){ return a.x<b.x?a:b; }
  float opS(float a, float b){ return max(a,-b); }
  // grid of shallow window recesses carved into the square-tier faces (domain repetition)
  float winCarve(vec3 p){
    float ry = mod(p.y + 0.13, 0.26) - 0.13;
    float rz = mod(p.z + 0.15, 0.30) - 0.15;
    float rx = mod(p.x + 0.15, 0.30) - 0.15;
    float wx = sdBox(vec3(abs(p.x)-0.80, ry, rz), vec3(0.14, 0.075, 0.055));
    float wz = sdBox(vec3(rx, ry, abs(p.z)-0.80), vec3(0.055, 0.075, 0.14));
    return min(wx, wz);
  }
  // octagon facet detail in ONE angular fold: shallow panel + deeper window rows
  float octCarve(vec3 p){
    float ang=atan(p.z,p.x); float sec=6.2831853/8.0;
    float k=mod(ang+0.5*sec, sec)-0.5*sec; float rad=length(p.xz);
    vec3 fp=vec3(rad*cos(k), p.y, rad*sin(k));
    float panel = sdBox(vec3(fp.x-0.50, p.y-2.86, fp.z), vec3(0.05,0.52,0.13));
    float ry=mod(p.y+0.12,0.24)-0.12;
    float win = sdBox(vec3(fp.x-0.54, ry, fp.z), vec3(0.09,0.06,0.06));
    return min(panel, win);
  }
  // recessed windows in the perimeter wall + corner bastions
  float wallCarve(vec3 p){
    float rz=mod(p.z+0.35,0.70)-0.35;
    float rx=mod(p.x+0.35,0.70)-0.35;
    float wx=sdBox(vec3(abs(p.x)-3.25, p.y+1.74, rz), vec3(0.10,0.15,0.15));
    float wz=sdBox(vec3(rx, p.y+1.74, abs(p.z)-2.45), vec3(0.15,0.15,0.10));
    return min(wx,wz);
  }
  float bastCarve(vec3 p){
    float ry=mod(p.y+0.42,0.52)-0.26;
    float bx=sdBox(vec3(abs(p.x)-3.40, ry, abs(p.z)-2.15), vec3(0.10,0.13,0.15));
    float bz=sdBox(vec3(abs(p.x)-2.95, ry, abs(p.z)-2.60), vec3(0.15,0.13,0.10));
    return min(bx,bz);
  }

  // vec2(distance, material)  1=stone 2=lantern 3=rock 4=bronze statue
  vec2 map(vec3 p){
    vec2 res = vec2(1e9, 0.0);
    res = opU(res, vec2(sdEllipsoid(p-vec3(0.0,-2.55,0.0), vec3(3.6,0.6,2.9)), 3.0));
    // sprawling walled compound
    res = opU(res, vec2(sdBox(p-vec3(0.0,-2.05,0.0), vec3(3.2,0.18,2.4)), 1.0));   // platform
    float wall = opS(sdBox(p-vec3(0.0,-1.74,0.0), vec3(3.25,0.44,2.45)),
                     sdBox(p-vec3(0.0,-1.70,0.0), vec3(3.02,0.62,2.22)));
    wall = opS(wall, wallCarve(p));                                                 // perimeter wall + window insets
    res = opU(res, vec2(wall, 1.0));
    float bast = opS(sdBox(vec3(abs(p.x)-2.95, p.y+1.42, abs(p.z)-2.15), vec3(0.45,0.82,0.45)), bastCarve(p));
    res = opU(res, vec2(bast, 1.0));                                                // 4 corner bastions + insets
    res = opU(res, vec2(sdBox(vec3(abs(p.x)-2.95, p.y+2.18, abs(p.z)-2.15), vec3(0.52,0.10,0.52)), 1.0)); // base step
    res = opU(res, vec2(sdBox(vec3(abs(p.x)-2.95, p.y+0.66, abs(p.z)-2.15), vec3(0.52,0.09,0.52)), 1.0)); // flared cap
    // stepped base
    res = opU(res, vec2(sdBox(p-vec3(0.0,-1.55,0.0), vec3(1.05,0.20,1.05)), 1.0));
    res = opU(res, vec2(sdBox(p-vec3(0.0,-1.22,0.0), vec3(0.85,0.16,0.85)), 1.0));
    // broad square tier 1 (~2x the octagon top width)
    res = opU(res, vec2(opS(taperSquare(p, -1.05, 2.05, 0.82, 0.72), winCarve(p)), 1.0));
    res = opU(res, vec2(sdBox(p-vec3(0.0,2.11,0.0), vec3(0.72,0.06,0.72)), 1.0));   // cornice sits flush on top (no overhang)
    // acroteria along all four top edges (flaring out); corners flare more
    float acroZ = max(sdBox(vec3(abs(p.x)-0.78, p.y-2.22, mod(p.z+0.24,0.24)-0.12), vec3(0.05,0.11,0.05)), abs(p.z)-0.70);
    float acroX = max(sdBox(vec3(mod(p.x+0.24,0.24)-0.12, p.y-2.22, abs(p.z)-0.78), vec3(0.05,0.11,0.05)), abs(p.x)-0.70);
    float acroC = sdBox(vec3(abs(p.x)-0.80, p.y-2.27, abs(p.z)-0.80), vec3(0.09,0.17,0.09));
    res = opU(res, vec2(min(min(acroZ,acroX),acroC), 5.0));
    // octagon tier 2 — recessed panel per facet, window inset within
    res = opU(res, vec2(opS(taperOct(p, 2.16, 3.62, 0.52, 0.42), octCarve(p)), 1.0));
    res = opU(res, vec2(sdBox(p-vec3(0.0,3.66,0.0), vec3(0.42,0.05,0.42)), 1.0));   // octagon cornice (flush)
    // open colonnade cupola: plinth + ring of pillars + fire + conical roof
    res = opU(res, vec2(sdCyl(p-vec3(0.0,3.76,0.0), 0.08, 0.33), 1.0));             // plinth
    res = opU(res, vec2(colonnade(p, 3.86, 4.42, 0.27, 0.04, 8.0), 1.0));           // pillars
    res = opU(res, vec2(sdCyl(p-vec3(0.0,4.14,0.0), 0.26, 0.15), 2.0));             // fire (emissive)
    res = opU(res, vec2(sdCone(p-vec3(0.0,4.60,0.0), 0.22, 0.33, 0.0), 1.0));       // pointier roof
    res = opU(res, vec2(length(p-vec3(0.0,4.86,0.0))-0.028, 1.0));                  // finial
    // statue — a person reaching up
    float legL = sdCapsule(p, vec3(0.045,4.86,0.0), vec3(0.03,5.12,0.0), 0.033);
    float legR = sdCapsule(p, vec3(-0.045,4.86,0.0), vec3(-0.03,5.12,0.0), 0.033);
    float torso= sdCapsule(p, vec3(0.0,5.10,0.0), vec3(0.005,5.36,0.0), 0.052);
    float head = length(p-vec3(0.01,5.43,0.0))-0.048;
    float armU = sdCapsule(p, vec3(0.02,5.32,0.0), vec3(0.15,5.58,0.02), 0.028);
    float armD = sdCapsule(p, vec3(-0.02,5.32,0.0), vec3(-0.12,5.17,0.03), 0.028);
    float statue = min(min(min(legL,legR),min(torso,head)),min(armU,armD));
    res = opU(res, vec2(statue, 4.0));
    return res;
  }

  // coarse silhouette (no carves / no angular folds) for cheap shadows + AO
  float mapCoarse(vec3 p){
    float d = sdEllipsoid(p-vec3(0.0,-2.55,0.0), vec3(3.6,0.6,2.9));
    d = min(d, sdBox(p-vec3(0.0,-2.05,0.0), vec3(3.2,0.18,2.4)));
    d = min(d, sdBox(p-vec3(0.0,-1.74,0.0), vec3(3.25,0.44,2.45)));
    d = min(d, sdBox(vec3(abs(p.x)-2.95, p.y+1.42, abs(p.z)-2.15), vec3(0.52,0.90,0.52)));
    d = min(d, sdBox(p-vec3(0.0,-1.38,0.0), vec3(1.0,0.36,1.0)));
    d = min(d, taperSquare(p, -1.05, 2.11, 0.82, 0.72));
    d = min(d, taperOct(p, 2.16, 3.66, 0.52, 0.42));
    d = min(d, sdCyl(p-vec3(0.0,4.12,0.0), 0.62, 0.34));
    d = min(d, sdCone(p-vec3(0.0,4.60,0.0), 0.22, 0.33, 0.0));
    d = min(d, sdCyl(p-vec3(0.0,5.15,0.0), 0.35, 0.09));
    return d;
  }

  vec3 calcNormal(vec3 p){
    vec2 e=vec2(0.0016,0.0);
    return normalize(vec3(
      map(p+e.xyy).x-map(p-e.xyy).x,
      map(p+e.yxy).x-map(p-e.yxy).x,
      map(p+e.yyx).x-map(p-e.yyx).x));
  }
  float hash(vec2 p){ return fract(sin(dot(p, vec2(41.3,289.1)))*43758.5453); }
  // iq soft shadow + ambient occlusion — what gives the recesses depth
  float softshadow(vec3 ro, vec3 rd, float mint, float maxt, float k){
    float res=1.0, t=mint;
    for(int i=0;i<26;i++){
      float h=mapCoarse(ro+rd*t);
      if(h<0.001) return 0.0;
      res=min(res, k*h/t);
      t+=clamp(h,0.02,0.28);
      if(t>maxt) break;
    }
    return clamp(res,0.0,1.0);
  }
  float calcAO(vec3 p, vec3 n){
    float occ=0.0, sca=1.0;
    for(int i=0;i<5;i++){
      float hr=0.02+0.13*float(i)/4.0;
      float dd=mapCoarse(p+n*hr);
      occ+=(hr-dd)*sca;
      sca*=0.7;
    }
    return clamp(1.0-2.0*occ,0.0,1.0);
  }
  float bayer2(vec2 a){ a=floor(a); return fract(a.x*0.5 + a.y*a.y*0.75); }
  float bayer4(vec2 a){ return bayer2(0.5*a)*0.25 + bayer2(a); }
  vec3 hsv2rgb(vec3 c){
    vec4 K=vec4(1.0,2.0/3.0,1.0/3.0,3.0);
    vec3 p=abs(fract(c.xxx+K.xyz)*6.0-K.www);
    return c.z*mix(K.xxx, clamp(p-K.xxx,0.0,1.0), c.y);
  }
  vec3 prism(float t){ return hsv2rgb(vec3(fract(t), 0.5, 1.0)); }

  void main(){
    vec2 uv = (2.0*gl_FragCoord.xy - u_res) / u_res.y;

    vec3 target = vec3(0.0, 1.55 + u_camH, 0.0);
    float cp=cos(u_pitch), sp=sin(u_pitch);
    vec3 ro = target + u_zoom*vec3(cp*sin(u_yaw), sp, cp*cos(u_yaw));
    vec3 f = normalize(target-ro);
    vec3 rr = normalize(cross(vec3(0.0,1.0,0.0), f));
    vec3 uu = cross(f, rr);
    vec3 rd = normalize(uv.x*rr + uv.y*uu + 1.7*f);

    float t=0.0, m=0.0;
    for(int i=0;i<120;i++){
      vec3 p=ro+rd*t; vec2 h=map(p);
      if(h.x<0.0013){ m=h.y; break; }
      t += h.x*0.8;
      if(t>30.0) break;
    }
    float tHit = (m>0.5)? t : 30.0;

    float L; vec3 col;
    if(m<0.5){
      float v=smoothstep(1.7,0.0,length(uv));
      col=mix(vec3(0.30,0.34,0.42), prism(0.72), 0.35);
      L=0.015+0.03*v;
    } else {
      vec3 p=ro+rd*t; vec3 n=calcNormal(p);
      vec3 lig=normalize(vec3(cos(u_lightEl)*sin(u_lightAz), sin(u_lightEl), cos(u_lightEl)*cos(u_lightAz)));
      float dif=clamp(dot(n,lig),0.0,1.0);
      float sh=(u_shadow>0.5)? softshadow(p+n*0.02, lig, 0.03, 7.0, 10.0) : 1.0;
      float ao=calcAO(p,n);
      float skyl=0.5+0.5*n.y;
      float fill=clamp(dot(n, normalize(vec3(-lig.x,0.25,-lig.z))),0.0,1.0);
      L = dif*sh*0.92 + skyl*0.20*ao + fill*0.12 + 0.08*ao;
      if(m>1.5 && m<2.5){                        // lantern fire — emissive prism
        L=(0.85+0.15*sin(u_time*2.0));
        col=prism(u_time*0.12+p.y*0.6);
      } else if(m>4.5){                          // acroteria — dark architectural stone
        col=vec3(0.28,0.30,0.36);
      } else if(m>3.5){                          // bronze statue
        col=vec3(0.80,0.63,0.34);
      } else if(m>2.5){                          // rock
        col=vec3(0.46,0.52,0.60); L*=0.8;
      } else {                                   // stone masonry
        col=vec3(0.82,0.85,0.92);
        L*=0.92+0.08*smoothstep(0.35,0.5,abs(fract(p.y*3.0)-0.5));   // faint course lines
      }
      L*=exp(-0.035*t);
    }

    if(u_beam>0.5){
      vec3 Lp=vec3(0.0,4.14,0.0);
      float ba=u_time*u_beamSpeed*u_beamDir;
      vec3 bd=normalize(vec3(sin(ba),-0.04,cos(ba)));
      vec3 w0=ro-Lp;
      float b=dot(rd,bd), d0=dot(rd,w0), e=dot(bd,w0);
      float denom=max(1.0-b*b, 1e-4);
      float sc=(b*e-d0)/denom, tc=(e-b*d0)/denom;
      float maxLen=75.0;
      float tcc=clamp(tc, 0.0, maxLen);
      vec3 Pc=ro+rd*max(sc,0.0), Qc=Lp+bd*tcc;
      float dseg=length(Pc-Qc);
      float width=0.12+0.014*tcc;
      float core=exp(-(dseg*dseg)/(2.0*width*width));
      float lenFade=smoothstep(maxLen, maxLen*0.55, tc);   // full length, soft only at the far end
      float occ=(m>0.5) ? step(sc, tHit) : 1.0;            // occluded by geometry, but never by the sky cap
      float glow=core*lenFade*step(0.0,tc)*step(0.0,sc)*occ;
      L += glow*1.25;
      vec3 beamCol = mix(prism(u_time*0.12 + 0.1), vec3(1.0,0.98,0.92), 0.35);
      col = mix(col, beamCol, clamp(glow*1.3,0.0,1.0));
    }

    gl_FragColor = vec4(col, clamp(L, 0.0, 1.0));
  }`;

  // Pass 2 — the "look": reads the scene luminance buffer and stylizes it.
  const FRAG_POST = `
  precision highp float;
  uniform vec2 u_res;
  uniform vec2 u_cell;
  uniform float u_nchars, u_grain, u_mode, u_time;
  uniform sampler2D u_scene;
  uniform sampler2D u_atlas;
  float hash(vec2 p){ return fract(sin(dot(p, vec2(41.3,289.1)))*43758.5453); }
  float bayer2(vec2 a){ a=floor(a); return fract(a.x*0.5 + a.y*a.y*0.75); }
  float bayer4(vec2 a){ return bayer2(0.5*a)*0.25 + bayer2(a); }
  void main(){
    vec2 cellIdx = floor(gl_FragCoord.xy / u_cell);
    vec2 sc = (u_mode < 0.5) ? (cellIdx + 0.5) * u_cell : gl_FragCoord.xy;
    vec4 scene = texture2D(u_scene, sc / u_res);
    vec3 col = scene.rgb;
    float L = scene.a;
    float g = hash(cellIdx + floor(u_time*24.0));
    L += (g-0.5) * u_grain * (0.3 + 0.7*L);
    L = clamp(L, 0.0, 1.0);

    if(u_mode < 0.5){                            // ASCII
      float gi=floor(L*(u_nchars-0.001));
      vec2 local=fract(gl_FragCoord.xy/u_cell);
      vec2 auv=vec2((gi+local.x)/u_nchars, local.y);
      float mask=texture2D(u_atlas, auv).r;
      gl_FragColor=vec4(col*mask, 1.0);
    } else if(u_mode < 1.5){                     // fine dither (Bayer + hash → organic stipple)
      float th = clamp(bayer4(gl_FragCoord.xy)*0.8 + hash(gl_FragCoord.xy)*0.2, 0.0, 1.0);
      gl_FragColor=vec4(col*step(th, L), 1.0);
    } else if(u_mode < 2.5){                     // plain shaded render
      gl_FragColor=vec4(col*L, 1.0);
    } else if(u_mode < 3.5){                     // scanlines (CRT-ish)
      float slc = 0.72 + 0.28*sin(gl_FragCoord.y*3.14159265);
      float vg = 0.92 + 0.08*sin(gl_FragCoord.x*1.5707963);
      gl_FragColor=vec4(col*L*slc*vg, 1.0);
    } else {                                     // ridgeline (scanline displacement, hidden-line removal)
      float px=gl_FragCoord.x, py=gl_FragCoord.y;
      float S = max(3.0, u_cell.y);
      float amp = 8.0 * S;
      float w = 1.15;
      float maxC = -1e9; float v = 0.0;
      float startY = floor((py - amp) / S) * S;
      for(int k=0;k<30;k++){
        float Yb = startY + float(k)*S;
        if(Yb > py) break;
        float Lc = texture2D(u_scene, vec2(px, Yb)/u_res).a;
        float yc = Yb + amp*Lc;
        if(yc > maxC){
          if(py <= yc && py >= maxC){ v = smoothstep(w, 0.0, abs(py - yc)); }
          maxC = yc;
        }
      }
      gl_FragColor = vec4(vec3(0.90,0.93,1.0)*v, 1.0);
    }
  }`;

  const RAMPS = [
    " .:-=+*oa%#@",
    " .'`^\",:;Il!i~+?][}{1)(|/#",
    " ·:+=x#▓█",
  ];

  class PharosAscii extends HTMLElement {
    constructor() {
      super();
      this._s = {
        cell: 10, mode: 0, grain: 0.15, beam: 1, beamSpeed: 0.5, orbitSpeed: 0.2, orbitDir: 1,
        elevation: 0.32, height: 0, drift: 0, zoom: 16, ramp: 0, auto: 1,
        lightAz: -0.6, lightEl: 0.7, shadow: 1,
      };
      this._yaw = 0.7; this._dragging = false;
    }

    static get observedAttributes() {
      return ["cell","mode","grain","beam","beam-speed","orbit-speed","orbit-dir","elevation","height","drift","zoom","ramp","auto","light-az","light-el","shadow","interactive"];
    }

    attributeChangedCallback(name, _o, v) {
      const map = { "beam-speed":"beamSpeed", "orbit-speed":"orbitSpeed", "orbit-dir":"orbitDir", "light-az":"lightAz", "light-el":"lightEl" };
      const key = map[name] || name;
      if (name === "interactive") { this._wireInput(); return; }
      if (v == null || v === "") return;
      const num = parseFloat(v);
      this._s[key] = isNaN(num) ? (v === "true" ? 1 : v === "false" ? 0 : this._s[key]) : num;
      if (name === "cell" || name === "ramp") this._buildAtlas();
    }

    _defineProps() {
      ["cell","mode","grain","beam","beamSpeed","orbitSpeed","orbitDir","elevation","height","drift","zoom","ramp","auto","lightAz","lightEl","shadow"].forEach((k) => {
        const pre = Object.getOwnPropertyDescriptor(this, k);
        let preVal;
        if (pre && "value" in pre) { preVal = pre.value; delete this[k]; }
        if (!Object.getOwnPropertyDescriptor(this, k)) {
          Object.defineProperty(this, k, {
            get() { return this._s[k]; },
            set(val) { const n = parseFloat(val); this._s[k] = isNaN(n) ? this._s[k] : n; if (k === "cell" || k === "ramp") this._buildAtlas(); },
          });
        }
        if (preVal !== undefined) this[k] = preVal;
      });
    }

    connectedCallback() {
      this._defineProps();
      this.style.display = this.style.display || "block";
      if (getComputedStyle(this).position === "static") this.style.position = "relative";
      this.style.overflow = "hidden";
      const c = document.createElement("canvas");
      c.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block;";
      this._canvas = c; this.appendChild(c);

      const gl = c.getContext("webgl", { antialias: false });
      if (!gl) { this.textContent = "WebGL unavailable"; return; }
      this._gl = gl;

      const sh = (type, src) => { const s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s);
        if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) console.error("pharos shader:", gl.getShaderInfoLog(s)); return s; };
      const link = (fsrc) => { const p = gl.createProgram(); gl.attachShader(p, sh(gl.VERTEX_SHADER, VERT)); gl.attachShader(p, sh(gl.FRAGMENT_SHADER, fsrc)); gl.linkProgram(p);
        if (!gl.getProgramParameter(p, gl.LINK_STATUS)) console.error("pharos link:", gl.getProgramInfoLog(p)); return p; };
      this._progScene = link(FRAG_SCENE);
      this._progPost = link(FRAG_POST);

      const buf = gl.createBuffer();
      gl.bindBuffer(gl.ARRAY_BUFFER, buf);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1, 3,-1, -1,3]), gl.STATIC_DRAW);
      this._buf = buf;
      this._aScene = gl.getAttribLocation(this._progScene, "a_pos");
      this._aPost = gl.getAttribLocation(this._progPost, "a_pos");

      this._Us = {};
      ["u_res","u_time","u_yaw","u_pitch","u_zoom","u_camH","u_beam","u_beamSpeed","u_beamDir","u_lightAz","u_lightEl","u_shadow"]
        .forEach((n) => this._Us[n] = gl.getUniformLocation(this._progScene, n));
      this._Up = {};
      ["u_res","u_cell","u_nchars","u_grain","u_mode","u_time","u_scene","u_atlas"]
        .forEach((n) => this._Up[n] = gl.getUniformLocation(this._progPost, n));

      this._fbo = gl.createFramebuffer();
      this._sceneTex = gl.createTexture();
      this._atlas = gl.createTexture();
      this._buildAtlas();
      if (document.fonts) document.fonts.ready.then(() => this._buildAtlas());

      this._ro = new ResizeObserver(() => this._resize()); this._ro.observe(this);
      this._resize();
      this._wireInput();

      this._last = performance.now(); this._acc = 0; this._frames = 0;
      this._raf = requestAnimationFrame(this._loop);
    }

    disconnectedCallback() {
      cancelAnimationFrame(this._raf);
      if (this._ro) this._ro.disconnect();
      this._unwireInput();
      const ext = this._gl && this._gl.getExtension("WEBGL_lose_context");
      if (ext) ext.loseContext();
    }

    _dpr() { return Math.min(window.devicePixelRatio || 1, 1.5); }
    _cellW() { return Math.max(4, Math.round(this._s.cell * 0.6)); }

    _buildAtlas() {
      if (!this._gl) return;
      const gl = this._gl;
      const ramp = RAMPS[Math.max(0, Math.min(RAMPS.length - 1, this._s.ramp | 0))];
      this._nchars = ramp.length;
      const dpr = this._dpr();
      const cw = Math.round(this._cellW() * dpr), ch = Math.round(this._s.cell * dpr);
      const cv = document.createElement("canvas");
      cv.width = cw * this._nchars; cv.height = ch;
      const x = cv.getContext("2d");
      x.clearRect(0, 0, cv.width, cv.height);
      x.fillStyle = "#fff";
      x.font = `500 ${Math.max(6, ch - 2)}px "Geist Mono", ui-monospace, Menlo, monospace`;
      x.textAlign = "center"; x.textBaseline = "middle";
      for (let i = 0; i < this._nchars; i++) x.fillText(ramp[i], i * cw + cw / 2, ch / 2 + 0.5);
      gl.bindTexture(gl.TEXTURE_2D, this._atlas);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, cv);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    }

    _resize() {
      const gl = this._gl; if (!gl) return;
      const dpr = this._dpr();
      const w = Math.max(1, Math.floor(this.clientWidth * dpr));
      const h = Math.max(1, Math.floor(this.clientHeight * dpr));
      if (this._canvas.width !== w || this._canvas.height !== h) { this._canvas.width = w; this._canvas.height = h; }
      gl.viewport(0, 0, w, h);
    }

    // scene buffer is downscaled to save raymarch work; finer when a full-detail
    // look (shaded / scanlines) is selected, coarse (~1 sample/cell) otherwise
    _ensureScene() {
      const gl = this._gl;
      const dpr = this._dpr();
      const fine = (this._s.mode === 2 || this._s.mode === 3);
      const q = (this._s.mode === 1) ? Math.max(1, Math.round(this._s.cell * dpr * 0.2))
              : fine ? Math.max(1, Math.round(this._s.cell * dpr * 0.22))
                     : Math.max(2, Math.round(this._s.cell * dpr * 0.5));
      const sw = Math.max(1, Math.ceil(this._canvas.width / q));
      const sh = Math.max(1, Math.ceil(this._canvas.height / q));
      if (sw === this._sw && sh === this._sh) return;
      this._sw = sw; this._sh = sh;
      gl.bindTexture(gl.TEXTURE_2D, this._sceneTex);
      gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, sw, sh, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    }

    _wireInput() {
      if (!this._canvas) return;
      const on = this.hasAttribute("interactive");
      this._unwireInput();
      if (!on) return;
      this._canvas.style.touchAction = "none";
      this._canvas.style.cursor = "grab";
      const down = (e) => { this._dragging = true; this._s.auto = 0; this._px = e.clientX; this._py = e.clientY; this._canvas.style.cursor = "grabbing"; this._canvas.setPointerCapture(e.pointerId); this.dispatchEvent(new CustomEvent("autochange", { detail: 0 })); };
      const move = (e) => { if (!this._dragging) return; this._yaw -= (e.clientX - this._px) * 0.006; this._s.elevation = Math.max(-0.35, Math.min(0.98, this._s.elevation + (e.clientY - this._py) * 0.004)); this._px = e.clientX; this._py = e.clientY; this.dispatchEvent(new CustomEvent("orbitchange", { detail: { elevation: this._s.elevation } })); };
      const up = () => { this._dragging = false; if (this._canvas) this._canvas.style.cursor = "grab"; };
      const wheel = (e) => { e.preventDefault(); this._s.zoom = Math.max(6, Math.min(30, this._s.zoom + e.deltaY * 0.02)); this.dispatchEvent(new CustomEvent("zoomchange", { detail: this._s.zoom })); };
      this._h = { down, move, up, wheel };
      this._canvas.addEventListener("pointerdown", down);
      this._canvas.addEventListener("pointermove", move);
      this._canvas.addEventListener("pointerup", up);
      this._canvas.addEventListener("pointercancel", up);
      this._canvas.addEventListener("wheel", wheel, { passive: false });
    }
    _unwireInput() {
      if (!this._canvas || !this._h) return;
      this._canvas.removeEventListener("pointerdown", this._h.down);
      this._canvas.removeEventListener("pointermove", this._h.move);
      this._canvas.removeEventListener("pointerup", this._h.up);
      this._canvas.removeEventListener("pointercancel", this._h.up);
      this._canvas.removeEventListener("wheel", this._h.wheel);
      this._h = null;
    }

    _loop = (now) => {
      const gl = this._gl; if (!gl) return;
      const dt = (now - this._last) / 1000; this._last = now;
      const s = this._s;
      if (s.auto && !this._dragging) this._yaw += dt * s.orbitSpeed * (s.orbitDir < 0 ? -1 : 1);
      const pitch = Math.max(-0.35, Math.min(0.98, s.elevation + s.drift * Math.sin(now * 0.0004)));
      const w = this._canvas.width, h = this._canvas.height, dpr = this._dpr();
      this._ensureScene();
      const sw = this._sw, sh = this._sh;

      // pass 1 — raymarch the scene into a downscaled luminance buffer
      gl.bindFramebuffer(gl.FRAMEBUFFER, this._fbo);
      gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, this._sceneTex, 0);
      gl.viewport(0, 0, sw, sh);
      gl.useProgram(this._progScene);
      gl.bindBuffer(gl.ARRAY_BUFFER, this._buf);
      gl.enableVertexAttribArray(this._aScene); gl.vertexAttribPointer(this._aScene, 2, gl.FLOAT, false, 0, 0);
      gl.uniform2f(this._Us.u_res, sw, sh);
      gl.uniform1f(this._Us.u_time, now / 1000);
      gl.uniform1f(this._Us.u_yaw, this._yaw);
      gl.uniform1f(this._Us.u_pitch, pitch);
      gl.uniform1f(this._Us.u_zoom, s.zoom);
      gl.uniform1f(this._Us.u_camH, s.height);
      gl.uniform1f(this._Us.u_beam, s.beam ? 1 : 0);
      gl.uniform1f(this._Us.u_beamSpeed, s.beamSpeed);
      gl.uniform1f(this._Us.u_beamDir, s.orbitDir < 0 ? -1 : 1);
      gl.uniform1f(this._Us.u_lightAz, s.lightAz);
      gl.uniform1f(this._Us.u_lightEl, s.lightEl);
      gl.uniform1f(this._Us.u_shadow, s.shadow ? 1 : 0);
      gl.drawArrays(gl.TRIANGLES, 0, 3);

      // pass 2 — the "look"
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.viewport(0, 0, w, h);
      gl.useProgram(this._progPost);
      gl.bindBuffer(gl.ARRAY_BUFFER, this._buf);
      gl.enableVertexAttribArray(this._aPost); gl.vertexAttribPointer(this._aPost, 2, gl.FLOAT, false, 0, 0);
      gl.uniform2f(this._Up.u_res, w, h);
      gl.uniform2f(this._Up.u_cell, this._cellW() * dpr, s.cell * dpr);
      gl.uniform1f(this._Up.u_nchars, this._nchars);
      gl.uniform1f(this._Up.u_grain, s.grain);
      gl.uniform1f(this._Up.u_mode, s.mode);
      gl.uniform1f(this._Up.u_time, now / 1000);
      gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, this._sceneTex); gl.uniform1i(this._Up.u_scene, 0);
      gl.activeTexture(gl.TEXTURE1); gl.bindTexture(gl.TEXTURE_2D, this._atlas); gl.uniform1i(this._Up.u_atlas, 1);
      gl.drawArrays(gl.TRIANGLES, 0, 3);

      this._acc += dt; this._frames++;
      if (this._acc >= 0.5) { this.dispatchEvent(new CustomEvent("fps", { detail: Math.round(this._frames / this._acc) })); this._acc = 0; this._frames = 0; }
      this._raf = requestAnimationFrame(this._loop);
    };

    get yaw() { return this._yaw; }
    set yaw(v) { const n = parseFloat(v); if (!isNaN(n)) this._yaw = n; }
  }

  customElements.define("pharos-ascii", PharosAscii);
})();
