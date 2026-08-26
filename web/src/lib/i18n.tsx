import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

export type Language = 'he' | 'en'

const STORAGE_KEY = 'blessedbot.language'

/** Dictionaries are plain JSON-shaped objects: the app ships two languages and
 *  a flat key space is easier to keep complete than nested namespaces. */
const dictionaries: Record<Language, Record<string, string>> = {
  en: {
    'app.title': 'Blessed by the Bot',
    'nav.dashboard': 'Dashboard',
    'nav.contacts': 'Contacts',
    'nav.blessings': 'Blessings',
    'nav.groups': 'Groups',
    'nav.settings': 'Settings',

    'common.add': 'Add',
    'common.edit': 'Edit',
    'common.delete': 'Delete',
    'common.save': 'Save',
    'common.cancel': 'Cancel',
    'common.close': 'Close',
    'common.search': 'Search',
    'common.enabled': 'Enabled',
    'common.disabled': 'Disabled',
    'common.language': 'Language',
    'common.name': 'Name',
    'common.actions': 'Actions',
    'common.all': 'All',
    'common.loading': 'Loading…',
    'common.retry': 'Retry',
    'common.none': 'None',
    'common.yes': 'Yes',
    'common.no': 'No',
    'common.saving': 'Saving…',
    'common.optional': 'optional',

    'notices.title': 'Needs your attention',
    'notices.dismiss': 'Dismiss',
    'notices.seenTimes': 'seen {n} times',
    'notices.language_fallback': 'Missing translation',
    'dashboard.provider': 'Provider',
    'dashboard.connected': 'Connected',
    'dashboard.disconnected': 'Not connected',
    'dashboard.needsQR': 'Needs QR scan',
    'dashboard.notConfigured': 'Not configured',
    'dashboard.configure': 'Configure it in Settings',
    'dashboard.upcoming': 'Upcoming events',
    'dashboard.upcomingEmpty': 'No events in the next 30 days.',
    'dashboard.recent': 'Recent sends',
    'dashboard.recentEmpty': 'Nothing has been sent yet.',
    'dashboard.stats': 'Today',
    'dashboard.sentToday': 'Sent today',
    'dashboard.failedToday': 'Failed today',
    'dashboard.contacts': 'Contacts',
    'dashboard.today': 'Today',
    'dashboard.tomorrow': 'Tomorrow',
    'dashboard.inDays': 'in {n} days',

    'contacts.title': 'Contacts',
    'contacts.add': 'Add contact',
    'contacts.empty': 'No contacts yet. Add someone to start sending blessings.',
    'contacts.noMatch': 'No contacts match your search.',
    'contacts.phone': 'Phone',
    'contacts.eventDate': 'Event date',
    'contacts.eventType': 'Event type',
    'contacts.relation': 'Relation',
    'contacts.gender': 'Gender',
    'contacts.importance': 'Importance',
    'contacts.sendTime': 'Send time',
    'contacts.customSendTime': 'Use a custom send time',
    'contacts.sendNow': 'Send now',
    'contacts.sendNowDone': 'Blessing sent.',
    'contacts.confirmDelete': 'Delete this contact?',
    'contacts.phoneHint': 'International format, digits only (e.g. 972501234567)',

    'blessings.title': 'Blessings',
    'blessings.add': 'Add blessing',
    'blessings.empty': 'No templates for this event type yet.',
    'blessings.text': 'Text',
    'blessings.preview': 'Preview',
    'blessings.targeting': 'Targeting',
    'blessings.anyGender': 'Any gender',
    'blessings.anyRelation': 'Any relation',
    'blessings.placeholderHint': 'Use {{name}} to insert the contact’s name.',
    'blessings.groupUsable': 'Usable for group echo',
    'blessings.groupUsableHint':
      'Templates without {{name}} can also be posted into groups.',
    'blessings.confirmDelete': 'Delete this blessing?',

    'groups.title': 'Groups',
    'groups.add': 'Add group',
    'groups.empty': 'No groups watched yet. Add one to join in when people celebrate.',
    'groups.chatId': 'Chat ID',
    'groups.threshold': 'Threshold',
    'groups.thresholdHint': 'Distinct people who must wish before the bot joins in.',
    'groups.useDefault': 'Use the global default',
    'groups.pick': 'Pick from WhatsApp',
    'groups.pickUnavailable': 'The provider cannot list groups; enter the chat ID manually.',
    'groups.manual': 'Enter manually',
    'groups.confirmDelete': 'Stop watching this group?',
    'groups.patterns': 'Wish patterns',
    'groups.patternsHint':
      'Phrases that make the bot notice a celebration. Wrap in /slashes/ for a regular expression.',
    'groups.addPattern': 'Add pattern',
    'groups.pattern': 'Pattern',

    'settings.title': 'Settings',
    'settings.provider': 'WhatsApp provider',
    'settings.greenapi': 'GreenAPI',
    'settings.gowa': 'GOWA (self-hosted)',
    'settings.apiUrl': 'API URL',
    'settings.idInstance': 'Instance ID',
    'settings.apiToken': 'API token',
    'settings.mode': 'Incoming messages',
    'settings.modePolling': 'Polling (works behind NAT)',
    'settings.modeWebhook': 'Webhook',
    'settings.webhookAuth': 'Webhook auth header',
    'settings.baseUrl': 'Base URL',
    'settings.username': 'Username',
    'settings.password': 'Password',
    'settings.deviceId': 'Device ID',
    'settings.webhookSecret': 'Webhook secret',
    'settings.webhookSecretHint':
      'Required. Webhooks without a valid signature are rejected.',
    'settings.scheduler': 'Scheduler',
    'settings.timezone': 'Timezone',
    'settings.generalSendTime': 'Default send time',
    'settings.groupEcho': 'Group echo',
    'settings.threshold': 'Threshold',
    'settings.window': 'Window (hours)',
    'settings.cooldown': 'Cooldown (hours)',
    'settings.test': 'Test connection',
    'settings.sendTest': 'Send test message',
    'settings.testPhone': 'Phone or chat ID',
    'settings.saved': 'Settings saved.',
    'settings.secretKept': 'Leave unchanged to keep the stored value.',
    'settings.uiLanguage': 'Interface language',
    'settings.theme': 'Theme',
    'settings.themeLight': 'Light',
    'settings.themeDark': 'Dark',
    'settings.themeSystem': 'System',

    'event.birthday': 'Birthday',
    'event.wedding': 'Wedding',
    'event.anniversary': 'Anniversary',
    'event.custom': 'Custom',
    'gender.male': 'Male',
    'gender.female': 'Female',
    'gender.other': 'Other',
    'relation.friend': 'Friend',
    'relation.close_friend': 'Close friend',
    'relation.family': 'Family',
    'relation.coworker': 'Coworker',
    'kind.scheduled': 'Scheduled',
    'kind.group_echo': 'Group echo',
    'status.sent': 'Sent',
    'status.failed': 'Failed',
  },
  he: {
    'app.title': 'מבורך על ידי הבוט',
    'nav.dashboard': 'לוח בקרה',
    'nav.contacts': 'אנשי קשר',
    'nav.blessings': 'ברכות',
    'nav.groups': 'קבוצות',
    'nav.settings': 'הגדרות',

    'common.add': 'הוספה',
    'common.edit': 'עריכה',
    'common.delete': 'מחיקה',
    'common.save': 'שמירה',
    'common.cancel': 'ביטול',
    'common.close': 'סגירה',
    'common.search': 'חיפוש',
    'common.enabled': 'פעיל',
    'common.disabled': 'כבוי',
    'common.language': 'שפה',
    'common.name': 'שם',
    'common.actions': 'פעולות',
    'common.all': 'הכול',
    'common.loading': 'טוען…',
    'common.retry': 'נסה שוב',
    'common.none': 'ללא',
    'common.yes': 'כן',
    'common.no': 'לא',
    'common.saving': 'שומר…',
    'common.optional': 'לא חובה',

    'notices.title': 'דורש טיפול',
    'notices.dismiss': 'סגירה',
    'notices.seenTimes': 'נראה {n} פעמים',
    'notices.language_fallback': 'חסר תרגום',
    'dashboard.provider': 'ספק',
    'dashboard.connected': 'מחובר',
    'dashboard.disconnected': 'לא מחובר',
    'dashboard.needsQR': 'נדרשת סריקת QR',
    'dashboard.notConfigured': 'לא מוגדר',
    'dashboard.configure': 'הגדירו בעמוד ההגדרות',
    'dashboard.upcoming': 'אירועים קרובים',
    'dashboard.upcomingEmpty': 'אין אירועים ב-30 הימים הקרובים.',
    'dashboard.recent': 'שליחות אחרונות',
    'dashboard.recentEmpty': 'עדיין לא נשלח דבר.',
    'dashboard.stats': 'היום',
    'dashboard.sentToday': 'נשלחו היום',
    'dashboard.failedToday': 'נכשלו היום',
    'dashboard.contacts': 'אנשי קשר',
    'dashboard.today': 'היום',
    'dashboard.tomorrow': 'מחר',
    'dashboard.inDays': 'בעוד {n} ימים',

    'contacts.title': 'אנשי קשר',
    'contacts.add': 'הוספת איש קשר',
    'contacts.empty': 'אין עדיין אנשי קשר. הוסיפו מישהו כדי להתחיל לשלוח ברכות.',
    'contacts.noMatch': 'אין אנשי קשר שתואמים לחיפוש.',
    'contacts.phone': 'טלפון',
    'contacts.eventDate': 'תאריך האירוע',
    'contacts.eventType': 'סוג אירוע',
    'contacts.relation': 'קשר',
    'contacts.gender': 'מגדר',
    'contacts.importance': 'חשיבות',
    'contacts.sendTime': 'שעת שליחה',
    'contacts.customSendTime': 'שעת שליחה מותאמת',
    'contacts.sendNow': 'שליחה עכשיו',
    'contacts.sendNowDone': 'הברכה נשלחה.',
    'contacts.confirmDelete': 'למחוק את איש הקשר?',
    'contacts.phoneHint': 'פורמט בינלאומי, ספרות בלבד (למשל 972501234567)',

    'blessings.title': 'ברכות',
    'blessings.add': 'הוספת ברכה',
    'blessings.empty': 'אין עדיין תבניות לסוג האירוע הזה.',
    'blessings.text': 'טקסט',
    'blessings.preview': 'תצוגה מקדימה',
    'blessings.targeting': 'התאמה',
    'blessings.anyGender': 'כל מגדר',
    'blessings.anyRelation': 'כל קשר',
    'blessings.placeholderHint': 'השתמשו ב-{{name}} כדי לשלב את שם איש הקשר.',
    'blessings.groupUsable': 'מתאימה לקבוצות',
    'blessings.groupUsableHint': 'תבניות ללא {{name}} יכולות להישלח גם לקבוצות.',
    'blessings.confirmDelete': 'למחוק את הברכה?',

    'groups.title': 'קבוצות',
    'groups.add': 'הוספת קבוצה',
    'groups.empty': 'אין קבוצות במעקב. הוסיפו קבוצה כדי להצטרף לברכות.',
    'groups.chatId': 'מזהה צ׳אט',
    'groups.threshold': 'סף',
    'groups.thresholdHint': 'כמה אנשים שונים צריכים לברך לפני שהבוט מצטרף.',
    'groups.useDefault': 'שימוש בברירת המחדל',
    'groups.pick': 'בחירה מוואטסאפ',
    'groups.pickUnavailable': 'הספק אינו יכול להציג קבוצות; הזינו מזהה צ׳אט ידנית.',
    'groups.manual': 'הזנה ידנית',
    'groups.confirmDelete': 'להפסיק לעקוב אחרי הקבוצה?',
    'groups.patterns': 'ביטויי ברכה',
    'groups.patternsHint':
      'ביטויים שגורמים לבוט לזהות חגיגה. עטפו ב-/לוכסנים/ לביטוי רגולרי.',
    'groups.addPattern': 'הוספת ביטוי',
    'groups.pattern': 'ביטוי',

    'settings.title': 'הגדרות',
    'settings.provider': 'ספק וואטסאפ',
    'settings.greenapi': 'GreenAPI',
    'settings.gowa': 'GOWA (אחסון עצמי)',
    'settings.apiUrl': 'כתובת API',
    'settings.idInstance': 'מזהה מופע',
    'settings.apiToken': 'טוקן API',
    'settings.mode': 'הודעות נכנסות',
    'settings.modePolling': 'תשאול (עובד מאחורי NAT)',
    'settings.modeWebhook': 'Webhook',
    'settings.webhookAuth': 'כותרת אימות ל-Webhook',
    'settings.baseUrl': 'כתובת בסיס',
    'settings.username': 'שם משתמש',
    'settings.password': 'סיסמה',
    'settings.deviceId': 'מזהה מכשיר',
    'settings.webhookSecret': 'סוד Webhook',
    'settings.webhookSecretHint': 'חובה. Webhook ללא חתימה תקפה נדחה.',
    'settings.scheduler': 'תזמון',
    'settings.timezone': 'אזור זמן',
    'settings.generalSendTime': 'שעת שליחה כללית',
    'settings.groupEcho': 'הצטרפות לקבוצות',
    'settings.threshold': 'סף',
    'settings.window': 'חלון (שעות)',
    'settings.cooldown': 'המתנה (שעות)',
    'settings.test': 'בדיקת חיבור',
    'settings.sendTest': 'שליחת הודעת בדיקה',
    'settings.testPhone': 'טלפון או מזהה צ׳אט',
    'settings.saved': 'ההגדרות נשמרו.',
    'settings.secretKept': 'השאירו ללא שינוי כדי לשמור על הערך הקיים.',
    'settings.uiLanguage': 'שפת הממשק',
    'settings.theme': 'ערכת נושא',
    'settings.themeLight': 'בהיר',
    'settings.themeDark': 'כהה',
    'settings.themeSystem': 'מערכת',

    'event.birthday': 'יום הולדת',
    'event.wedding': 'חתונה',
    'event.anniversary': 'יום נישואין',
    'event.custom': 'מותאם',
    'gender.male': 'זכר',
    'gender.female': 'נקבה',
    'gender.other': 'אחר',
    'relation.friend': 'חבר',
    'relation.close_friend': 'חבר קרוב',
    'relation.family': 'משפחה',
    'relation.coworker': 'עמית לעבודה',
    'kind.scheduled': 'מתוזמן',
    'kind.group_echo': 'הצטרפות לקבוצה',
    'status.sent': 'נשלח',
    'status.failed': 'נכשל',
  },
}

interface I18nValue {
  language: Language
  setLanguage: (l: Language) => void
  t: (key: string, vars?: Record<string, string | number>) => string
  dir: 'rtl' | 'ltr'
}

const I18nContext = createContext<I18nValue | null>(null)

function initialLanguage(): Language {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'he' || stored === 'en') return stored
  } catch {
    // Private browsing or blocked storage: fall through to the default.
  }
  return navigator.language?.startsWith('he') ? 'he' : 'en'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(initialLanguage)
  const dir: 'rtl' | 'ltr' = language === 'he' ? 'rtl' : 'ltr'

  useEffect(() => {
    document.documentElement.lang = language
    document.documentElement.dir = dir
  }, [language, dir])

  const setLanguage = useCallback((l: Language) => {
    setLanguageState(l)
    try {
      localStorage.setItem(STORAGE_KEY, l)
    } catch {
      // Preference simply will not persist; the UI still works.
    }
  }, [])

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const raw = dictionaries[language][key] ?? dictionaries.en[key] ?? key
      if (!vars) return raw
      return Object.entries(vars).reduce(
        (acc, [name, value]) => acc.replaceAll(`{${name}}`, String(value)),
        raw,
      )
    },
    [language],
  )

  const value = useMemo(() => ({ language, setLanguage, t, dir }), [language, setLanguage, t, dir])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used inside I18nProvider')
  return ctx
}
