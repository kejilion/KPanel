// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { automaticCinemaMatch, cinemaSearchTitle, cinemaMetadataImage, cinemaSourceURL, readCinemaMetadataCache, saveCinemaMetadataCache, type CinemaMetadata } from './cinemaMetadata'
const movie: CinemaMetadata = { provider: 'tmdb', id: 27205, kind: 'movie', title: '盗梦空间', originalTitle: 'Inception', year: '2010', overview: 'A dream', poster: '/p.jpg', url: 'https://www.themoviedb.org/movie/27205' }
beforeEach(() => localStorage.clear())
describe('cinema source matching', () => {
  it('extracts title/year/episode without sending paths', () => {
    expect(cinemaSearchTitle('Inception.2010.1080p.BluRay.x264.mkv')).toEqual({ title: 'Inception', year: '2010', kind: 'movie' })
    expect(cinemaSearchTitle('The.Expanse.S01E02.1080p.mkv')).toEqual({ title: 'The Expanse', year: undefined, kind: 'tv' })
  })
  it('requires an unambiguous exact title and matching year', () => {
    expect(automaticCinemaMatch('Inception.2010.mkv', [movie])).toEqual(movie)
    expect(automaticCinemaMatch('Inception.2011.mkv', [movie])).toBeUndefined()
    expect(automaticCinemaMatch('Inception.mkv', [movie, { ...movie, id: 2 }])).toBeUndefined()
    expect(automaticCinemaMatch('Inception.mkv', [{ ...movie, title: 'Inception sequel', originalTitle: '' }])).toBeUndefined()
  })
  it('preserves numeric titles and uses the separate release year', () => {
    expect(cinemaSearchTitle('Blade.Runner.2049.2017.mkv')).toMatchObject({ title: 'Blade Runner 2049', year: '2017' })
    expect(cinemaSearchTitle('2001.A.Space.Odyssey.1968.mkv')).toMatchObject({ title: '2001 A Space Odyssey', year: '1968' })
    expect(cinemaSearchTitle('1917.2019.mkv')).toMatchObject({ title: '1917', year: '2019' })
    expect(cinemaSearchTitle('1917.mkv')).toMatchObject({ title: '1917', year: undefined })
    expect(cinemaSearchTitle('Blade.Runner.2049.mkv')).toMatchObject({ title: 'Blade Runner 2049', year: undefined })
  })
  it('uses same-origin artwork and constructs trusted source links', () => {
    expect(cinemaMetadataImage(movie)).toContain('/api/v1/cinema/image?provider=tmdb&path=%2Fp.jpg&size=poster')
    expect(cinemaSourceURL({ ...movie, url: 'javascript:evil' })).toBe('https://www.themoviedb.org/movie/27205')
    expect(cinemaSourceURL({ ...movie, provider: 'wikipedia', url: 'javascript:evil' })).toBeUndefined()
  })
  it('bounds disposable browser metadata and tolerates damaged storage', () => {
    const cache = new Map(Array.from({ length: 250 }, (_, i) => [`key${i}`, movie]))
    expect(saveCinemaMetadataCache(cache)).toBe(true)
    expect(readCinemaMetadataCache().size).toBe(200)
    localStorage.setItem('kpanel:cinema-metadata:v1', '{bad')
    expect(readCinemaMetadataCache().size).toBe(0)
    localStorage.setItem('kpanel:cinema-metadata:v1', JSON.stringify([{ key: 'expired', metadata: movie, expires: 1 }]))
    expect(readCinemaMetadataCache().size).toBe(0)
  })
})
