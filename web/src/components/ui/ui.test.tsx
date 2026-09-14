import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';

describe('ui primitives: static components render', () => {
  it('renders button, badge, card, input, skeleton, separator, table, tabs', () => {
    render(
      <div>
        <Button>Click me</Button>
        <Badge>New</Badge>
        <Card>
          <CardHeader>
            <CardTitle>Title</CardTitle>
          </CardHeader>
          <CardContent>Body</CardContent>
        </Card>
        <Input placeholder="type here" />
        <Skeleton data-testid="skeleton" />
        <Separator />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Col</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell>Val</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <Tabs defaultValue="a">
          <TabsList>
            <TabsTrigger value="a">A</TabsTrigger>
            <TabsTrigger value="b">B</TabsTrigger>
          </TabsList>
          <TabsContent value="a">Panel A</TabsContent>
          <TabsContent value="b">Panel B</TabsContent>
        </Tabs>
      </div>
    );

    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument();
    expect(screen.getByText('New')).toBeInTheDocument();
    expect(screen.getByText('Title')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('type here')).toBeInTheDocument();
    expect(screen.getByTestId('skeleton')).toBeInTheDocument();
    expect(screen.getByText('Col')).toBeInTheDocument();
    expect(screen.getByText('Panel A')).toBeInTheDocument();
  });
});

describe('Button variants', () => {
  it('applies outline variant classes', () => {
    render(<Button variant="outline">Outline</Button>);
    expect(screen.getByRole('button', { name: 'Outline' })).toHaveClass(
      'border', 'border-line-strong', 'bg-surface',
    );
  });

  it('applies secondary variant classes', () => {
    render(<Button variant="secondary">Secondary</Button>);
    expect(screen.getByRole('button', { name: 'Secondary' })).toHaveClass(
      'bg-raised', 'text-fg', 'border', 'border-line',
    );
  });
});

describe('Dialog', () => {
  it('opens on trigger click and closes on close button click', async () => {
    const user = userEvent.setup();
    render(
      <Dialog>
        <DialogTrigger asChild>
          <Button>Open dialog</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Dialog title</DialogTitle>
            <DialogDescription>Dialog description</DialogDescription>
          </DialogHeader>
          <p>Dialog body</p>
        </DialogContent>
      </Dialog>
    );

    expect(screen.queryByText('Dialog title')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Open dialog' }));
    expect(await screen.findByText('Dialog title')).toBeInTheDocument();
    expect(screen.getByText('Dialog body')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Close' }));
    await screen.findByText('Open dialog');
    expect(screen.queryByText('Dialog title')).not.toBeInTheDocument();
  });
});

describe('Popover', () => {
  it('renders a closed trigger without crashing', () => {
    render(
      <Popover>
        <PopoverTrigger asChild>
          <Button>Open popover</Button>
        </PopoverTrigger>
        <PopoverContent>Popover content</PopoverContent>
      </Popover>
    );

    expect(screen.getByRole('button', { name: 'Open popover' })).toBeInTheDocument();
    expect(screen.queryByText('Popover content')).not.toBeInTheDocument();
  });
});

describe('Tooltip', () => {
  it('renders trigger without crashing', () => {
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button>Hover me</Button>
          </TooltipTrigger>
          <TooltipContent>Tip text</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );

    expect(screen.getByRole('button', { name: 'Hover me' })).toBeInTheDocument();
  });
});

describe('DropdownMenu', () => {
  it('renders a closed trigger without crashing', () => {
    render(
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button>Menu</Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem>Item one</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );

    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument();
    expect(screen.queryByText('Item one')).not.toBeInTheDocument();
  });
});

describe('Select', () => {
  it('renders the selected value without crashing', () => {
    render(
      <Select defaultValue="one">
        <SelectTrigger aria-label="Choose">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="one">One</SelectItem>
          <SelectItem value="two">Two</SelectItem>
        </SelectContent>
      </Select>
    );

    expect(screen.getByRole('combobox')).toBeInTheDocument();
  });
});

describe('Command', () => {
  it('filters items by search input', async () => {
    const user = userEvent.setup();
    render(
      <Command>
        <CommandInput placeholder="Search..." />
        <CommandList>
          <CommandEmpty>No results</CommandEmpty>
          <CommandGroup heading="Servers">
            <CommandItem value="Denver">Denver</CommandItem>
            <CommandItem value="Seattle">Seattle</CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    );

    expect(screen.getByText('Denver')).toBeInTheDocument();
    expect(screen.getByText('Seattle')).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText('Search...'), 'Den');
    expect(screen.getByText('Denver')).toBeInTheDocument();
    expect(screen.queryByText('Seattle')).not.toBeInTheDocument();
  });
});
