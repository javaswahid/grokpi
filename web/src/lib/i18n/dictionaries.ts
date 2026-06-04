import { id } from './id'

export type Language = 'id' | 'en'
export type Dictionary = typeof id

export const defaultDictionary = id

export async function loadDictionary(language: Language): Promise<Dictionary> {
  if (language === 'en') {
    const mod = await import('./en')
    return mod.en
  }

  return id
}
