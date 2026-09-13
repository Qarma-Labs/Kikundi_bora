// Role-based navigation & permissions for Kikundi.
import { Home, Users, PiggyBank, Banknote, Receipt, FileBarChart2, User as UserIcon, ShieldCheck, Wallet, ClipboardList, UserPlus, Heart, Settings, Clock, Activity, FileCheck, Crown, Bell, Landmark, Briefcase, BookOpen, CalendarDays, HandCoins, HeartHandshake } from "lucide-react";
import type { Jukumu, LeadershipRole } from "@/api/types";

export interface NavItem {
  to: string;
  label: string;
  icon: any;
}

// Member navigation (all members see this)
export const memberNav: NavItem[] = [
  { to: "/dashibodi", label: "Dashboard Yangu", icon: Home },
  { to: "/deni-langu", label: "Deni Langu", icon: Wallet },
  { to: "/michango-yangu", label: "Michango Yangu", icon: PiggyBank },
  { to: "/weka-mchango", label: "Weka Mchango", icon: Banknote },
  { to: "/marejesho", label: "Marejesho", icon: Receipt },
  { to: "/mikopo", label: "Mikopo Yangu", icon: Banknote },
  { to: "/historia-yangu", label: "Historia Yangu", icon: Receipt },
  { to: "/mfuko-kijamii", label: "Mfuko wa Kijamii", icon: Heart },
];

// Leadership navigation (only for users with leadership roles)
// Each item can specify required roles — if omitted, all leadership roles see it.
export const leadershipNav: (NavItem & { requiredRoles?: LeadershipRole[] })[] = [
  { to: "/michango", label: "Pokea Michango", icon: PiggyBank, requiredRoles: ["HAZINA", "KATIBU"] },
  { to: "/taarifa-za-marejesho", label: "Taarifa Za Marejesho", icon: Receipt },
  { to: "/uongozi/mikopo", label: "Idhinisha Mikopo", icon: ShieldCheck },
  { to: "/uongozi/portfolio", label: "Portfolio ya Mikopo", icon: Briefcase },
  { to: "/njia-za-malipo", label: "Njia za Malipo", icon: Landmark, requiredRoles: ["MWENYEKITI", "HAZINA"] },
  { to: "/uongozi/ripoti", label: "Ripoti za Kikundi", icon: FileBarChart2 },
  { to: "/mikutano", label: "Mikutano na Makosa", icon: CalendarDays, requiredRoles: ["MWENYEKITI", "KATIBU"] },
  { to: "/ukusanyaji", label: "Ukusanyaji", icon: HandCoins, requiredRoles: ["HAZINA"] },
  { to: "/uongozi/mfuko", label: "Mfuko (Uongozi)", icon: HeartHandshake, requiredRoles: ["MWENYEKITI", "KATIBU", "HAZINA"] },
  { to: "/kitabu", label: "Kitabu cha Fedha", icon: BookOpen },
  { to: "/uongozi/import-data", label: "Ingiza Data", icon: FileCheck, requiredRoles: ["MWENYEKITI", "HAZINA"] },
  { to: "/wanachama", label: "Wanachama Wote", icon: Users },
];

// Legacy role-based nav (kept for backward compatibility)
export const sidebarNav: Record<Jukumu, NavItem[]> = {
  Mwenyekiti: [
    { to: "/dashibodi", label: "Dashibodi", icon: Home },
    { to: "/kamati-mikopo", label: "Kamati ya Mikopo", icon: UserPlus },
    { to: "/mfuko-kijamii", label: "Mfuko wa Kijamii", icon: Heart },
    { to: "/njia-za-malipo", label: "Njia za Malipo", icon: Landmark },
    { to: "/wanachama", label: "Wanachama", icon: Users },
    { to: "/mikopo", label: "Idhinisha Mikopo", icon: ShieldCheck },
    { to: "/vitendo-vinavyosubiri", label: "Vitendo Vinavyosubiri", icon: FileCheck },
    { to: "/taarifa-za-marejesho", label: "Taarifa Za Marejesho", icon: Receipt },
    { to: "/ripoti", label: "Ripoti", icon: FileBarChart2 },
    { to: "/mikutano", label: "Mikutano na Makosa", icon: CalendarDays },
    { to: "/kitabu", label: "Kitabu cha Fedha", icon: BookOpen },
  ],
  "Mweka Hazina": [
    { to: "/dashibodi", label: "Dashibodi", icon: Home },
    { to: "/kamati-mikopo", label: "Kamati ya Mikopo", icon: UserPlus },
    { to: "/mfuko-kijamii", label: "Mfuko wa Kijamii", icon: Heart },
    { to: "/njia-za-malipo", label: "Njia za Malipo", icon: Landmark },
    { to: "/michango", label: "Pokea Michango", icon: PiggyBank },
    { to: "/taarifa-za-marejesho", label: "Taarifa Za Marejesho", icon: Receipt },
    { to: "/mikopo", label: "Mikopo", icon: Wallet },
    { to: "/wanachama", label: "Wanachama", icon: Users },
    { to: "/ripoti", label: "Ripoti za Fedha", icon: FileBarChart2 },
    { to: "/ukusanyaji", label: "Ukusanyaji", icon: HandCoins },
    { to: "/uongozi/mfuko", label: "Mfuko (Usimamizi)", icon: HeartHandshake },
    { to: "/kitabu", label: "Kitabu cha Fedha", icon: BookOpen },
  ],
  Katibu: [
    { to: "/dashibodi", label: "Dashibodi", icon: Home },
    { to: "/kamati-mikopo", label: "Kamati ya Mikopo", icon: UserPlus },
    { to: "/mfuko-kijamii", label: "Mfuko wa Kijamii", icon: Heart },
    { to: "/wanachama", label: "Sajili Wanachama", icon: ClipboardList },
    { to: "/wanachama-kusubiri", label: "Wanaosubiri", icon: Clock },
    { to: "/michango", label: "Kumbukumbu", icon: PiggyBank },
    { to: "/mikopo", label: "Mikopo", icon: Banknote },
    { to: "/ripoti", label: "Ripoti", icon: FileBarChart2 },
    { to: "/mikutano", label: "Mikutano na Makosa", icon: CalendarDays },
    { to: "/kitabu", label: "Kitabu cha Fedha", icon: BookOpen },
  ],
  Mwanachama: [
    { to: "/dashibodi", label: "Akaunti Yangu", icon: Home },
    { to: "/deni-langu", label: "Deni Langu", icon: Wallet },
    { to: "/mfuko-kijamii", label: "Mfuko wa Kijamii", icon: Heart },
    { to: "/michango", label: "Michango Yangu", icon: PiggyBank },
    { to: "/mikopo", label: "Mikopo Yangu", icon: Banknote },
    { to: "/marejesho", label: "Marejesho", icon: Receipt },
  ],
  Msimamizi: [
    { to: "/dashibodi", label: "Dashibodi", icon: Home },
    { to: "/admin", label: "Simamia Watumiaji", icon: Settings },
    { to: "/admin-logs", label: "Kumbukumbu za Mfumo", icon: Activity },
  ],
};

// Dual plane navigation: returns member nav + leadership nav (if applicable)
// Admin (Msimamizi) is a system-level role — NOT a group member and NOT
// group leadership — so it gets only the admin nav (no member items, no
// leadership items, no group-operational links).
export function getDualPlaneNav(
  jukumu: Jukumu,
  isCommitteeMember: boolean,
  leadership: LeadershipRole[] = []
): { member: NavItem[]; leadership: NavItem[] } {
  if (jukumu === "Msimamizi") {
    return { member: [...sidebarNav.Msimamizi], leadership: [] };
  }
  const member = [...memberNav];
  
  // Filter leadership nav based on user's actual leadership roles
  const leadershipItems: NavItem[] = leadership.length > 0
    ? leadershipNav.filter((item) => {
        if (!item.requiredRoles) return true;
        return item.requiredRoles.some((r) => leadership.includes(r));
      }).map(({ to, label, icon }) => ({ to, label, icon }))
    : [];
  
  // Inject committee + bodi items for ordinary committee members
  if (jukumu === "Mwanachama" && isCommitteeMember) {
    member.push({ to: "/kamati-mikopo", label: "Kamati ya Mikopo", icon: UserPlus });
    // Bodi ya Mikopo member: show Idhinisha Mikopo in leadership section
    leadershipItems.push({ to: "/uongozi/mikopo", label: "Idhinisha Mikopo", icon: ShieldCheck });
  }
  
  return { member, leadership: leadershipItems };
}

// Legacy: Returns sidebar nav with committee item injected for ordinary committee members
export function getSidebarNav(jukumu: Jukumu, isCommitteeMember: boolean): NavItem[] {
  const items = [...sidebarNav[jukumu]];
  if (jukumu === "Mwanachama" && isCommitteeMember) {
    // Insert Kamati ya Mikopo after the first item (Dashibodi)
    items.splice(1, 0, { to: "/kamati-mikopo", label: "Kamati ya Mikopo", icon: UserPlus });
  }
  return items;
}

// Mobile bottom nav — keep 5 max, last is profile
export function mobileNav(jukumu: Jukumu, isCommitteeMember: boolean): NavItem[] {
  const items = getSidebarNav(jukumu, isCommitteeMember).slice(0, 4);
  return [...items, { to: "/wasifu", label: "Wasifu", icon: UserIcon }];
}

export const roleSubtitle: Record<Jukumu, string> = {
  Mwenyekiti: "Idhinisha mikopo na fuatilia kikundi",
  "Mweka Hazina": "Simamia fedha, michango na marejesho",
  Katibu: "Kumbukumbu na usajili wa wanachama",
  Mwanachama: "Akiba, mikopo na malipo yako",
  Msimamizi: "Simamia mfumo wote na watumiaji",
};

// Leadership role labels (Swahili)
export const leadershipRoleLabel: Record<LeadershipRole, string> = {
  MWENYEKITI: "Mwenyekiti",
  HAZINA: "Mweka Hazina",
  KATIBU: "Katibu",
};
