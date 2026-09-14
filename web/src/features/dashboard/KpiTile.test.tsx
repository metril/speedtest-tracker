import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { KpiTile } from './KpiTile';

it('renders the label and value', () => {
  render(<KpiTile label="Avg download" value="100.0 Mbps" />);
  expect(screen.getByText('Avg download')).toBeInTheDocument();
  expect(screen.getByText('100.0 Mbps')).toBeInTheDocument();
});

it('hides the delta when none is given', () => {
  render(<KpiTile label="Avg download" value="100.0 Mbps" />);
  expect(screen.queryByText(/%/)).not.toBeInTheDocument();
});

it('colors a rise as favorable in green for a throughput metric', () => {
  render(<KpiTile label="Avg download" value="110.0 Mbps" delta={0.1} favorable />);
  const badge = screen.getByLabelText(/up 10% from previous period/i);
  expect(badge).toHaveClass('text-ok');
});

it('colors a rise as unfavorable in red for a ping metric', () => {
  render(<KpiTile label="Avg ping" value="15.0 ms" delta={0.2} favorable={false} />);
  const badge = screen.getByLabelText(/up 20% from previous period/i);
  expect(badge).toHaveClass('text-bad');
});

it('colors a drop as favorable for a ping metric', () => {
  render(<KpiTile label="Avg ping" value="8.0 ms" delta={-0.25} favorable={false} />);
  const badge = screen.getByLabelText(/down 25% from previous period/i);
  expect(badge).toHaveClass('text-ok');
});
